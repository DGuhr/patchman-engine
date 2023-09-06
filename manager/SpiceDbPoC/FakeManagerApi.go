package SpiceDbPoC

import "net/http/httptest"

type FakeManagerApi struct {
	Server *httptest.Server
	// URI of the api
	URI string
}

// TODO: define mock endpoints that call either real handlers or mocked handlers call spiceDB.
