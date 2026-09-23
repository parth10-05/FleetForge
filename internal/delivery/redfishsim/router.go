// Package redfishsim owns the public simulator listener boundary.
// Route-dependent Redfish resources are intentionally absent until ADR-039 and
// the pinned QEMU/OpenBMC capability fixture establish their exact contract.
package redfishsim

import "net/http"

func Routes() http.Handler { return http.MaxBytesHandler(http.NewServeMux(), 4<<10) }
