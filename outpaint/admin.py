#!/usr/bin/env python3
"""Authenticated review CLI; don't put credentials on the command line."""

import argparse, json, pathlib, requests

p = argparse.ArgumentParser()
p.add_argument("--config", required=True)
p.add_argument("action", choices=["list", "approve", "reject"])
p.add_argument("id", nargs="?")
a = p.parse_args()
c = json.loads(pathlib.Path(a.config).expanduser().read_text())
s = requests.Session()
s.trust_env = False
url = c["nas_url"].rstrip("/") + "/v1/outpaint/jobs"
headers = {"Authorization": "Bearer " + c["token"]}
if a.action == "list":
    r = s.get(url, headers=headers, timeout=20)
else:
    if not a.id:
        p.error("job ID required")
    r = s.post(
        url + "/" + a.id + "/decision",
        headers=headers,
        json={"status": "approved" if a.action == "approve" else "rejected"},
        timeout=20,
    )
r.raise_for_status()
print(json.dumps(r.json(), indent=2))
