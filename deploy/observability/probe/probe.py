"""Independently measure API -> container logs -> Loki delivery.
No credentials/payloads are exported. HTTP endpoints are internal only.
"""
import json
import os
import threading
import time
import urllib.parse
import urllib.request
import uuid
from http.server import BaseHTTPRequestHandler, HTTPServer

API = os.environ.get('API_URL', 'http://server:8080/api/v1/public/ping')
LOKI = os.environ.get('LOKI_URL', 'http://loki:3100')
ENV = os.environ.get('LOG_ENV', 'local')
INSTANCE = os.environ.get('LOG_INSTANCE_ID', 'compose-server')
state = dict(api_up=0, loki_up=0, delivery_up=0, delivery_delay_seconds=0, last_check_seconds=0, jobs_up=0)
lock = threading.Lock()

def check_once():
    req_id = uuid.uuid4().hex
    started = time.time()
    result = dict(api_up=0, loki_up=0, delivery_up=0, delivery_delay_seconds=0, last_check_seconds=started)
    try:
        with urllib.request.urlopen(urllib.request.Request(API, headers={'X-Request-Id': req_id}), timeout=5) as response:
            result['api_up'] = int(response.status == 200)
    except Exception:
        pass
    # A query selector uses JSON encoding, never unescaped environment input.
    selector = '{service="fix-pro-api",env=' + json.dumps(ENV) + '}'
    query = selector + ' | json | __error__="" | requestId=' + json.dumps(req_id)
    deadline = started + 60
    while time.time() < deadline:
        try:
            params = urllib.parse.urlencode(dict(query=query, start=int(started * 1e9), end=int(time.time() * 1e9), limit=10))
            with urllib.request.urlopen(LOKI + '/loki/api/v1/query_range?' + params, timeout=5) as response:
                body = json.load(response)
            result['loki_up'] = int(body.get('status') == 'success')
            if body.get('data', {}).get('result'):
                result['delivery_up'] = 1
                break
        except Exception:
            result['loki_up'] = 0
        time.sleep(2)
    result['delivery_delay_seconds'] = time.time() - started
    try:
        now = time.time()
        q = selector + ' | json | __error__="" | instanceId=' + json.dumps(INSTANCE) + ' | event=~"job.completed|job.failed"'
        params = urllib.parse.urlencode(dict(query=q, start=int((now - 300) * 1e9), end=int(now * 1e9), limit=1000))
        with urllib.request.urlopen(LOKI + '/loki/api/v1/query_range?' + params, timeout=5) as response:
            body = json.load(response)
        seen = set()
        for stream in body.get('data', {}).get('result', []):
            for _, line in stream.get('values', []):
                event = json.loads(line)
                seen.add(event.get('jobName'))
        result['jobs_up'] = int({'avatar_cleanup', 'auto_recall', 'auto_acceptance'}.issubset(seen))
    except Exception:
        result['jobs_up'] = 0
    with lock:
        state.update(result)

def worker():
    while True:
        check_once()
        time.sleep(15)

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ('/metrics', '/health'):
            self.send_error(404)
            return
        with lock:
            values = dict(state)
        payload = '\n'.join('fixpro_log_probe_' + key + ' ' + str(value) for key, value in values.items()) + '\n'
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain; version=0.0.4')
        self.end_headers()
        self.wfile.write(payload.encode())
    def log_message(self, *_):
        pass

if __name__ == '__main__':
    threading.Thread(target=worker, daemon=True).start()
    HTTPServer(('0.0.0.0', 9098), Handler).serve_forever()
