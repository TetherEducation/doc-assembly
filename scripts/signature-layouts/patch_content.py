#!/usr/bin/env python3
"""
patch_content.py — Patches a PortableDocument JSON content structure so that
signerRoles have non-empty name/email values (required for template publishing).

Usage:
  python3 patch_content.py <input.json> [output.json]

If output.json is omitted, writes to stdout.
"""

import json
import sys
import os


def patch(data: dict) -> dict:
    """Patch signerRoles to have test-safe name and email values."""
    for role in data.get("signerRoles", []):
        # Only patch if the value is empty (leave injectable types alone)
        name_field = role.get("name", {})
        if name_field.get("type") == "text" and not name_field.get("value", "").strip():
            role_label = role.get("label", "Signer")
            role["name"] = {"type": "text", "value": role_label}

        email_field = role.get("email", {})
        if email_field.get("type") == "text" and not email_field.get("value", "").strip():
            # Use role index to generate a distinct email per role
            role_order = role.get("order", 1)
            role["email"] = {"type": "text", "value": f"signer-{role_order}@example.test"}

    return data


def main():
    if len(sys.argv) < 2:
        print("Usage: patch_content.py <input.json> [output.json]", file=sys.stderr)
        sys.exit(1)

    input_path = sys.argv[1]
    output_path = sys.argv[2] if len(sys.argv) > 2 else None

    with open(input_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    patched = patch(data)

    out = json.dumps(patched, ensure_ascii=False, separators=(",", ":"))

    if output_path:
        with open(output_path, "w", encoding="utf-8") as f:
            f.write(out)
        print(f"Patched: {input_path} -> {output_path}", file=sys.stderr)
    else:
        print(out)


if __name__ == "__main__":
    main()
