#!/usr/bin/env python3

"""
GitHub Organization Compliance Check
Checks GitHub organization settings for security compliance
"""

import json
import os
import sys
import requests
from datetime import datetime, timedelta
from typing import List, Dict, Any

class GitHubComplianceChecker:
    def __init__(self, token: str, org: str):
        self.token = token
        self.org = org
        self.base_url = "https://api.github.com"
        self.headers = {
            "Authorization": f"token {token}",
            "Accept": "application/vnd.github.v3+json",
            "User-Agent": "Openlane-Agent/1.0"
        }
        self.findings = []
        self.metrics = {}
        
    def add_finding(self, resource: str, title: str, description: str, 
                   severity: str, details: Dict[str, Any] = None):
        """Add a compliance finding"""
        finding = {
            "resource": resource,
            "title": title,
            "description": description,
            "severity": severity,
            "status": "open",
            "details": details or {}
        }
        self.findings.append(finding)
        
    def make_request(self, endpoint: str) -> Dict[str, Any]:
        """Make authenticated request to GitHub API"""
        url = f"{self.base_url}{endpoint}"
        response = requests.get(url, headers=self.headers)
        
        if response.status_code == 404:
            raise Exception(f"Resource not found: {endpoint}")
        elif response.status_code == 403:
            raise Exception(f"Insufficient permissions for: {endpoint}")
        elif response.status_code != 200:
            raise Exception(f"API request failed: {response.status_code} - {response.text}")
            
        return response.json()
        
    def check_organization_settings(self):
        """Check organization-level security settings"""
        print("Checking organization settings...", file=sys.stderr)
        
        try:
            org_data = self.make_request(f"/orgs/{self.org}")
        except Exception as e:
            self.add_finding(
                f"github:org:{self.org}",
                "Organization Access Error",
                f"Cannot access organization settings: {str(e)}",
                "critical",
                {"error": str(e)}
            )
            return
            
        # Check if two-factor authentication is required
        if not org_data.get("two_factor_requirement_enabled", False):
            self.add_finding(
                f"github:org:{self.org}",
                "Two-Factor Authentication Not Required",
                "Organization does not require 2FA for all members",
                "high",
                {"two_factor_requirement_enabled": False}
            )
            
        # Check default repository permissions
        default_repo_permission = org_data.get("default_repository_permission", "read")
        if default_repo_permission in ["write", "admin"]:
            self.add_finding(
                f"github:org:{self.org}",
                "Overly Permissive Default Repository Permission",
                f"Default repository permission is '{default_repo_permission}', should be 'read'",
                "medium",
                {"default_repository_permission": default_repo_permission}
            )
            
        # Check if members can create repositories
        members_can_create_repos = org_data.get("members_can_create_repositories", True)
        if members_can_create_repos:
            self.add_finding(
                f"github:org:{self.org}",
                "Members Can Create Repositories",
                "Organization allows all members to create repositories",
                "low",
                {"members_can_create_repositories": True}
            )
            
    def check_organization_members(self):
        """Check organization members and their permissions"""
        print("Checking organization members...", file=sys.stderr)
        
        try:
            members = self.make_request(f"/orgs/{self.org}/members?per_page=100")
            admins = self.make_request(f"/orgs/{self.org}/members?role=admin&per_page=100")
        except Exception as e:
            self.add_finding(
                f"github:org:{self.org}:members",
                "Member Access Error", 
                f"Cannot access member information: {str(e)}",
                "medium",
                {"error": str(e)}
            )
            return
            
        total_members = len(members)
        total_admins = len(admins)
        
        # Check admin ratio
        if total_members > 0:
            admin_ratio = total_admins / total_members
            if admin_ratio > 0.2:  # More than 20% admins might be excessive
                self.add_finding(
                    f"github:org:{self.org}:members",
                    "High Administrator Ratio",
                    f"{total_admins} out of {total_members} members are admins ({admin_ratio:.1%})",
                    "medium",
                    {
                        "total_members": total_members,
                        "total_admins": total_admins,
                        "admin_ratio": admin_ratio
                    }
                )
                
        self.metrics.update({
            "total_members": total_members,
            "total_admins": total_admins,
            "admin_ratio": admin_ratio if total_members > 0 else 0
        })
        
    def check_repositories(self):
        """Check repository security settings"""
        print("Checking repositories...", file=sys.stderr)
        
        try:
            repos = self.make_request(f"/orgs/{self.org}/repos?per_page=100&sort=updated")
        except Exception as e:
            self.add_finding(
                f"github:org:{self.org}:repos",
                "Repository Access Error",
                f"Cannot access repository information: {str(e)}",
                "medium", 
                {"error": str(e)}
            )
            return
            
        public_repos = 0
        private_repos = 0
        archived_repos = 0
        repos_without_branch_protection = 0
        
        for repo in repos[:20]:  # Check first 20 repos to avoid rate limits
            repo_name = repo["name"]
            
            if repo["visibility"] == "public":
                public_repos += 1
            else:
                private_repos += 1
                
            if repo["archived"]:
                archived_repos += 1
                continue
                
            # Check branch protection on main/master branch
            default_branch = repo["default_branch"]
            try:
                protection = self.make_request(f"/repos/{self.org}/{repo_name}/branches/{default_branch}/protection")
                # If we get here, branch protection exists
            except Exception:
                # Branch protection doesn't exist
                repos_without_branch_protection += 1
                self.add_finding(
                    f"github:repo:{self.org}/{repo_name}",
                    "No Branch Protection",
                    f"Repository {repo_name} does not have branch protection on {default_branch}",
                    "medium",
                    {
                        "repository": repo_name,
                        "default_branch": default_branch,
                        "has_branch_protection": False
                    }
                )
                
        self.metrics.update({
            "total_repositories": len(repos),
            "public_repositories": public_repos,
            "private_repositories": private_repos,
            "archived_repositories": archived_repos,
            "repos_without_branch_protection": repos_without_branch_protection
        })
        
    def check_webhooks(self):
        """Check organization webhooks"""
        print("Checking webhooks...", file=sys.stderr)
        
        try:
            webhooks = self.make_request(f"/orgs/{self.org}/hooks")
        except Exception as e:
            # Webhooks might not be accessible with current permissions
            return
            
        insecure_webhooks = 0
        
        for webhook in webhooks:
            config = webhook.get("config", {})
            url = config.get("url", "")
            insecure_ssl = config.get("insecure_ssl", False)
            
            if insecure_ssl or url.startswith("http://"):
                insecure_webhooks += 1
                self.add_finding(
                    f"github:org:{self.org}:webhook:{webhook['id']}",
                    "Insecure Webhook",
                    f"Webhook {webhook['id']} uses insecure connection",
                    "medium",
                    {
                        "webhook_id": webhook["id"],
                        "url": url,
                        "insecure_ssl": insecure_ssl
                    }
                )
                
        self.metrics.update({
            "total_webhooks": len(webhooks),
            "insecure_webhooks": insecure_webhooks
        })
        
    def run_checks(self) -> Dict[str, Any]:
        """Run all compliance checks"""
        print(f"Starting GitHub compliance check for organization: {self.org}", file=sys.stderr)
        
        try:
            self.check_organization_settings()
            self.check_organization_members() 
            self.check_repositories()
            self.check_webhooks()
        except Exception as e:
            self.add_finding(
                f"github:org:{self.org}",
                "Compliance Check Error",
                f"Error during compliance check: {str(e)}",
                "critical",
                {"error": str(e)}
            )
            
        # Prepare evidence
        evidence = {
            "organization": self.org,
            "check_time": datetime.utcnow().isoformat() + "Z",
            "github_api_version": "v3",
            "rate_limit_remaining": self.get_rate_limit()
        }
        
        return {
            "findings": self.findings,
            "evidence": evidence, 
            "metrics": self.metrics
        }
        
    def get_rate_limit(self) -> int:
        """Get remaining API rate limit"""
        try:
            response = requests.get(f"{self.base_url}/rate_limit", headers=self.headers)
            if response.status_code == 200:
                return response.json()["rate"]["remaining"]
        except:
            pass
        return 0

def main():
    # Get configuration from environment
    github_token = os.getenv("GITHUB_TOKEN")
    org_name = os.getenv("GITHUB_ORG")
    
    # Allow override from command line
    if len(sys.argv) > 1:
        org_name = sys.argv[1]
        
    if not github_token:
        result = {
            "error": "GITHUB_TOKEN environment variable is required"
        }
        print(json.dumps(result, indent=2))
        sys.exit(1)
        
    if not org_name:
        result = {
            "error": "GitHub organization name is required (set GITHUB_ORG or pass as argument)"
        }
        print(json.dumps(result, indent=2))
        sys.exit(1)
        
    # Run compliance checks
    checker = GitHubComplianceChecker(github_token, org_name)
    result = checker.run_checks()
    
    # Output results
    print(json.dumps(result, indent=2))
    
    print(f"GitHub compliance check completed. Found {len(result['findings'])} issues.", file=sys.stderr)

if __name__ == "__main__":
    main()