import http from 'k6/http';
import { check } from 'k6';

export const options = {
  vus: 2000,
  duration: '60s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<100'],
  },
};

export default function () {
  const res = http.get('http://127.0.0.1:8080/');
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
