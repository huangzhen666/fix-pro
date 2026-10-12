import io
import json
import unittest
from unittest.mock import patch
import probe

class ProbeTests(unittest.TestCase):
    def test_request_to_log_and_job_liveness(self):
        ids = []
        def response(url, **_):
            if isinstance(url, probe.urllib.request.Request):
                ids.append(url.get_header('X-request-id'))
                out = io.BytesIO(b'{}')
                out.status = 200
                return out
            query = probe.urllib.parse.parse_qs(probe.urllib.parse.urlparse(url).query)['query'][0]
            if 'requestId=' in query:
                self.assertIn(ids[0], query)
                body = {'status': 'success', 'data': {'result': [{'values': [['1', '{}']]}]}}
            else:
                body = {'data': {'result': [{'values': [['1', json.dumps({'jobName': name})] for name in ['avatar_cleanup', 'auto_recall', 'auto_acceptance']]}]}}
            return io.BytesIO(json.dumps(body).encode())
        with patch.object(probe.urllib.request, 'urlopen', side_effect=response):
            probe.check_once()
        self.assertEqual(probe.state['api_up'], 1)
        self.assertEqual(probe.state['delivery_up'], 1)
        self.assertEqual(probe.state['jobs_up'], 1)
    def test_failed_api_is_distinct_from_missing_logs(self):
        def response(url, **_):
            if isinstance(url, probe.urllib.request.Request):
                raise OSError('offline')
            return io.BytesIO(b'{"data":{"result":[]}}')
        with patch.object(probe.urllib.request, 'urlopen', side_effect=response), patch.object(probe.time, 'time', side_effect=[0, 61, 62, 63]):
            probe.check_once()
        self.assertEqual(probe.state['api_up'], 0)
        self.assertEqual(probe.state['delivery_up'], 0)
        self.assertEqual(probe.state['jobs_up'], 0)

if __name__ == '__main__':
    unittest.main()
