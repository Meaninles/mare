#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
import traceback

from cloud115_core import dispatch_bridge_operation, sanitize_error_payload


def main() -> int:
    request = json.load(sys.stdin)
    response = dispatch_bridge_operation(request)
    json.dump(response, sys.stdout, ensure_ascii=False)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as error:
        payload = sanitize_error_payload(error)
        payload["error"]["traceback"] = traceback.format_exc()
        json.dump(payload, sys.stdout, ensure_ascii=False)
        sys.exit(1)
