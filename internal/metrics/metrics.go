package metrics

import (
	"strconv"
	"strings"
	"sync/atomic"
)

type Metrics struct {
	Requests  atomic.Uint64
	Created   atomic.Uint64
	Conflicts atomic.Uint64
	Confirmed atomic.Uint64
	Failures  atomic.Uint64
}

func (m *Metrics) Render() string {
	return strings.Join([]string{
		"# HELP flashsale_http_requests_total Total HTTP requests.",
		"# TYPE flashsale_http_requests_total counter",
		"flashsale_http_requests_total " + strconv.FormatUint(m.Requests.Load(), 10),
		"# HELP flashsale_reservations_created_total Successful reservations.",
		"# TYPE flashsale_reservations_created_total counter",
		"flashsale_reservations_created_total " + strconv.FormatUint(m.Created.Load(), 10),
		"# HELP flashsale_reservation_conflicts_total Reservations rejected because the seat was taken.",
		"# TYPE flashsale_reservation_conflicts_total counter",
		"flashsale_reservation_conflicts_total " + strconv.FormatUint(m.Conflicts.Load(), 10),
		"# HELP flashsale_reservations_confirmed_total Confirmed reservations.",
		"# TYPE flashsale_reservations_confirmed_total counter",
		"flashsale_reservations_confirmed_total " + strconv.FormatUint(m.Confirmed.Load(), 10),
		"# HELP flashsale_request_failures_total Invalid or failed requests.",
		"# TYPE flashsale_request_failures_total counter",
		"flashsale_request_failures_total " + strconv.FormatUint(m.Failures.Load(), 10),
		"",
	}, "\n")
}
