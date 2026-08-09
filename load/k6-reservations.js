import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    reservations: {
      executor: 'constant-vus',
      vus: 100,
      duration: '30s',
    },
  },
};

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  const userID = `load-user-${__VU}-${__ITER}`;
  const seatID = `seat-${(__VU % 100) + 1}`;
  const response = http.post(
    `${baseURL}/v1/events/flash-test/reservations`,
    JSON.stringify({ user_id: userID, seat_id: seatID }),
    {
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': `${userID}-${seatID}`,
      },
    },
  );

  check(response, {
    'reservation response is accepted or conflict': (res) => res.status === 201 || res.status === 409,
  });
}
