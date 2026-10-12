"""Generate source lookup for the same SQL fingerprint used by pgx tracer.
Only source file and line are written; no SQL or query parameters are exported.
Runtime-built SQL may have no static entry; use the operation field then.
"""
from pathlib import Path
import hashlib
import json
import re
root = Path(__file__).resolve().parent.parent / 'apps/server-go'
result = {}
for path in (root / 'internal').rglob('*.go'):
    if path.name.endswith('_test.go'):
        continue
    source = path.read_text()
    for match in re.finditer(r'`([^`]+)`', source):
        query = match.group(1)
        if query.lstrip().split(' ', 1)[0].upper() not in ('SELECT', 'INSERT', 'UPDATE', 'DELETE'):
            continue
        name = 'sql_' + hashlib.sha256(query.encode()).hexdigest()[:12]
        result[name] = {'file': str(path.relative_to(root)), 'line': source[:match.start()].count('\n') + 1}
(root / 'api/diagnostic-queries.json').write_text(json.dumps(result, indent=2, sort_keys=True) + '\n')
print(f'Generated {len(result)} diagnostic query source references')
