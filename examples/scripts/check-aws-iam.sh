#!/bin/bash

# AWS IAM Compliance Check
# Checks for common IAM security issues like missing MFA, old access keys, etc.

set -euo pipefail

# Configuration
MAX_KEY_AGE_DAYS=${MAX_KEY_AGE_DAYS:-90}
REQUIRE_MFA=${REQUIRE_MFA:-true}

# Initialize findings array
findings=()
evidence={}
metrics={}

# Function to add a finding
add_finding() {
    local resource="$1"
    local title="$2"
    local description="$3"
    local severity="$4"
    local details="$5"
    
    finding=$(cat <<EOF
{
    "resource": "$resource",
    "title": "$title", 
    "description": "$description",
    "severity": "$severity",
    "status": "open",
    "details": $details
}
EOF
    )
    
    findings+=("$finding")
}

# Check if AWS CLI is available
if ! command -v aws &> /dev/null; then
    echo '{"error": "AWS CLI not found. Please install the AWS CLI."}' >&2
    exit 1
fi

# Check AWS credentials
if ! aws sts get-caller-identity &> /dev/null; then
    echo '{"error": "AWS credentials not configured or invalid."}' >&2
    exit 1
fi

echo "Checking AWS IAM compliance..." >&2

# Get account ID for context
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
echo "Account ID: $ACCOUNT_ID" >&2

# Initialize counters
total_users=0
users_without_mfa=0
old_access_keys=0
unused_access_keys=0

# Check IAM users
echo "Checking IAM users..." >&2
while IFS=$'\t' read -r username create_date password_last_used; do
    total_users=$((total_users + 1))
    
    # Check for MFA devices
    mfa_devices=$(aws iam list-mfa-devices --user-name "$username" --query 'MFADevices[*].SerialNumber' --output text)
    
    if [[ "$REQUIRE_MFA" == "true" && -z "$mfa_devices" ]]; then
        users_without_mfa=$((users_without_mfa + 1))
        add_finding \
            "arn:aws:iam::$ACCOUNT_ID:user/$username" \
            "User without MFA" \
            "IAM user $username does not have MFA enabled" \
            "high" \
            "{\"user_name\": \"$username\", \"mfa_devices\": []}"
    fi
    
    # Check access keys
    while IFS=$'\t' read -r access_key_id status create_date_key last_used; do
        if [[ "$status" == "Active" ]]; then
            # Calculate key age
            if command -v date &> /dev/null; then
                if [[ "$OSTYPE" == "darwin"* ]]; then
                    # macOS date command
                    key_age_days=$(( ($(date +%s) - $(date -j -f "%Y-%m-%dT%H:%M:%S" "${create_date_key%+*}" +%s)) / 86400 ))
                else
                    # Linux date command
                    key_age_days=$(( ($(date +%s) - $(date -d "$create_date_key" +%s)) / 86400 ))
                fi
                
                if [[ $key_age_days -gt $MAX_KEY_AGE_DAYS ]]; then
                    old_access_keys=$((old_access_keys + 1))
                    add_finding \
                        "arn:aws:iam::$ACCOUNT_ID:user/$username" \
                        "Old Access Key" \
                        "Access key $access_key_id is $key_age_days days old (max: $MAX_KEY_AGE_DAYS)" \
                        "medium" \
                        "{\"user_name\": \"$username\", \"access_key_id\": \"$access_key_id\", \"age_days\": $key_age_days}"
                fi
            fi
            
            # Check if key has been used recently
            if [[ "$last_used" == "None" || "$last_used" == "" ]]; then
                unused_access_keys=$((unused_access_keys + 1))
                add_finding \
                    "arn:aws:iam::$ACCOUNT_ID:user/$username" \
                    "Unused Access Key" \
                    "Access key $access_key_id has never been used" \
                    "low" \
                    "{\"user_name\": \"$username\", \"access_key_id\": \"$access_key_id\", \"last_used\": \"never\"}"
            fi
        fi
    done < <(aws iam list-access-keys --user-name "$username" --query 'AccessKeyMetadata[*].[AccessKeyId,Status,CreateDate]' --output text | while read -r key_id status create_date_key; do
        # Get last used info
        last_used=$(aws iam get-access-key-last-used --access-key-id "$key_id" --query 'AccessKeyLastUsed.LastUsedDate' --output text 2>/dev/null || echo "None")
        echo -e "$key_id\t$status\t$create_date_key\t$last_used"
    done)
    
done < <(aws iam list-users --query 'Users[*].[UserName,CreateDate,PasswordLastUsed]' --output text)

# Check account password policy
echo "Checking password policy..." >&2
if ! aws iam get-account-password-policy &> /dev/null; then
    add_finding \
        "arn:aws:iam::$ACCOUNT_ID:account-password-policy" \
        "No Password Policy" \
        "AWS account does not have a password policy configured" \
        "high" \
        "{\"policy_exists\": false}"
else
    # Check password policy strength
    min_length=$(aws iam get-account-password-policy --query 'PasswordPolicy.MinimumPasswordLength' --output text)
    require_symbols=$(aws iam get-account-password-policy --query 'PasswordPolicy.RequireSymbols' --output text)
    require_numbers=$(aws iam get-account-password-policy --query 'PasswordPolicy.RequireNumbers' --output text)
    require_uppercase=$(aws iam get-account-password-policy --query 'PasswordPolicy.RequireUppercaseCharacters' --output text)
    require_lowercase=$(aws iam get-account-password-policy --query 'PasswordPolicy.RequireLowercaseCharacters' --output text)
    
    if [[ "$min_length" -lt 14 ]]; then
        add_finding \
            "arn:aws:iam::$ACCOUNT_ID:account-password-policy" \
            "Weak Password Length" \
            "Password minimum length is $min_length, should be at least 14 characters" \
            "medium" \
            "{\"minimum_length\": $min_length, \"recommended\": 14}"
    fi
    
    if [[ "$require_symbols" != "True" || "$require_numbers" != "True" || 
          "$require_uppercase" != "True" || "$require_lowercase" != "True" ]]; then
        add_finding \
            "arn:aws:iam::$ACCOUNT_ID:account-password-policy" \
            "Weak Password Complexity" \
            "Password policy should require symbols, numbers, uppercase and lowercase characters" \
            "medium" \
            "{\"require_symbols\": \"$require_symbols\", \"require_numbers\": \"$require_numbers\", \"require_uppercase\": \"$require_uppercase\", \"require_lowercase\": \"$require_lowercase\"}"
    fi
fi

# Prepare metrics
metrics_json=$(cat <<EOF
{
    "total_users": $total_users,
    "users_without_mfa": $users_without_mfa,
    "old_access_keys": $old_access_keys,
    "unused_access_keys": $unused_access_keys,
    "mfa_compliance_rate": $(echo "scale=2; ($total_users - $users_without_mfa) * 100 / $total_users" | bc -l 2>/dev/null || echo "0")
}
EOF
)

# Prepare evidence
evidence_json=$(cat <<EOF
{
    "account_id": "$ACCOUNT_ID",
    "check_time": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "aws_cli_version": "$(aws --version 2>&1 | head -n1)",
    "parameters": {
        "max_key_age_days": $MAX_KEY_AGE_DAYS,
        "require_mfa": "$REQUIRE_MFA"
    }
}
EOF
)

# Build findings array JSON
if [[ ${#findings[@]} -gt 0 ]]; then
    findings_json="[$(IFS=,; echo "${findings[*]}")]"
else
    findings_json="[]"
fi

# Output final JSON
cat <<EOF
{
    "findings": $findings_json,
    "evidence": $evidence_json,
    "metrics": $metrics_json
}
EOF

echo "AWS IAM compliance check completed. Found ${#findings[@]} issues." >&2