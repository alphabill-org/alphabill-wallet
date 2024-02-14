package args

import "testing"

func TestBuildRpcUrl(t *testing.T) {
	testCases := []struct {
		inputURL    string
		expectedURL string
	}{
		// base cases
		{"127.0.0.1:1000", "http://127.0.0.1:1000/rpc"},
		{"127.0.0.1:1000/", "http://127.0.0.1:1000/rpc"},

		// http cases
		{"http://127.0.0.1:1000", "http://127.0.0.1:1000/rpc"},
		{"http://127.0.0.1:1000/", "http://127.0.0.1:1000/rpc"},
		{"http://127.0.0.1:1000/rpc", "http://127.0.0.1:1000/rpc"},
		{"http://127.0.0.1:1000/rpc/", "http://127.0.0.1:1000/rpc"},

		// https cases
		{"https://127.0.0.1:1000", "https://127.0.0.1:1000/rpc"},
		{"https://127.0.0.1:1000/", "https://127.0.0.1:1000/rpc"},
		{"https://127.0.0.1:1000/rpc", "https://127.0.0.1:1000/rpc"},
		{"https://127.0.0.1:1000/rpc/", "https://127.0.0.1:1000/rpc"},
	}

	for _, tc := range testCases {
		processedURL := BuildRpcUrl(tc.inputURL)
		if processedURL != tc.expectedURL {
			t.Errorf("expected %s, but got %s for input %s", tc.expectedURL, processedURL, tc.inputURL)
		}
	}
}
