#!/usr/bin/env python3
"""External hook: deny operations on sensitive environment/config files."""
import json
import sys
import os


SENSITIVE_PATTERNS = [".env", ".pem", "id_rsa", "id_ed25519", "credentials.json", "secrets.yaml"]


def is_sensitive(file_path: str) -> bool:
    base = os.path.basename(file_path)
    for pattern in SENSITIVE_PATTERNS:
        if base == pattern or base.endswith(pattern):
            return True
    return False


def main():
    try:
        request = json.load(sys.stdin)
    except json.JSONDecodeError:
        print(json.dumps({"decision": "deny", "reason": "invalid stdin JSON"}))
        sys.exit(2)

    tool = request.get("input", {}).get("tool", "")
    args = request.get("output", {}).get("args", {})
    file_path = args.get("file_path", "")

    if tool in ("write_file", "view") and file_path and is_sensitive(file_path):
        print(json.dumps({"decision": "deny", "reason": f"sensitive file blocked by env-protection: {os.path.basename(file_path)}"}))
        sys.exit(2)

    print(json.dumps({"decision": "allow"}))
    sys.exit(0)


if __name__ == "__main__":
    main()
