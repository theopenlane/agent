#!/bin/bash
echo "Testing compliance check execution..."
echo "Check: $OPENLANE_CHECK_NAME"
echo "Controls: $OPENLANE_CONTROLS"

# Output compliance findings in JSON format
cat << JSON
{
  "findings": [
    {
      "resource": "test-resource",
      "title": "Test Finding",
      "description": "This is a test compliance finding",
      "severity": "medium", 
      "status": "open",
      "details": {
        "test": true,
        "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      }
    }
  ],
  "evidence": {
    "test_execution": true,
    "script_path": "$0"
  },
  "metrics": {
    "execution_time_ms": 100,
    "resources_checked": 1
  }
}
JSON