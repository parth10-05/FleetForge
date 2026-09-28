package redfish

import (
	"context"
	"encoding/json"
)

// ServiceCapabilities is the deliberately small v1 discovery result. Links
// remain service-root-relative inputs and must be resolved again before use.
type ServiceCapabilities struct {
	RedfishVersion   string
	SystemsURI       string
	ManagersURI      string
	UpdateServiceURI string
}

// DiscoverService performs only a read of the fixed service root. It never
// follows action links or submits firmware.
func (client *Client) DiscoverService(ctx context.Context, address TargetAddress) (ServiceCapabilities, error) {
	raw, err := client.GetDocument(ctx, address, "")
	if err != nil {
		return ServiceCapabilities{}, err
	}
	var document struct {
		RedfishVersion string `json:"RedfishVersion"`
		Systems        struct {
			ODataID string `json:"@odata.id"`
		} `json:"Systems"`
		Managers struct {
			ODataID string `json:"@odata.id"`
		} `json:"Managers"`
		UpdateService struct {
			ODataID string `json:"@odata.id"`
		} `json:"UpdateService"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || document.RedfishVersion == "" || document.Systems.ODataID == "" || document.Managers.ODataID == "" || document.UpdateService.ODataID == "" {
		return ServiceCapabilities{}, DiscoveryError{Kind: ErrorMalformed, err: ErrMalformedDocument}
	}
	for _, uri := range []string{document.Systems.ODataID, document.Managers.ODataID, document.UpdateService.ODataID} {
		if _, err := address.ResolveResource(uri); err != nil {
			return ServiceCapabilities{}, DiscoveryError{Kind: ErrorMalformed, err: err}
		}
	}
	return ServiceCapabilities{RedfishVersion: document.RedfishVersion, SystemsURI: document.Systems.ODataID, ManagersURI: document.Managers.ODataID, UpdateServiceURI: document.UpdateService.ODataID}, nil
}
