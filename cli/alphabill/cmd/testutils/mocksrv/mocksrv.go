package mocksrv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
)

type (
	BackendMockReturnConf struct {
		Balance        uint64
		BlockHeight    uint64
		TargetBill     *wallet.Bill
		FeeCreditBill  *wallet.Bill
		ProofList      string
		CustomBillList string
		CustomPath     string
		CustomFullPath string
		CustomResponse string
	}
)

func MockBackendCalls(br *BackendMockReturnConf) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.CustomPath || r.URL.RequestURI() == br.CustomFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.CustomResponse))
		} else {
			path := r.URL.Path
			switch {
			case path == "/"+client.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.Balance)))
			case path == "/"+client.RoundNumberPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.BlockHeight)))
			case path == "/api/v1/units/":
				if br.ProofList != "" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(br.ProofList))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			case path == "/"+client.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.CustomBillList != "" {
					w.Write([]byte(br.CustomBillList))
				} else {
					b, _ := json.Marshal(br.TargetBill)
					w.Write([]byte(fmt.Sprintf(`{"bills": [%s]}`, b)))
				}
			case strings.Contains(path, client.FeeCreditPath):
				w.WriteHeader(http.StatusOK)
				fcb, _ := json.Marshal(br.FeeCreditBill)
				w.Write(fcb)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}
