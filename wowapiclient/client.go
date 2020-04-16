package wowapiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	_apiHostFormat = "%s.api.blizzard.com"

	_defaultTimeout = time.Second * 10
)

// WOWAPIClient provides access to the WOW API given an HTTP Client and a Region
type WOWAPIClient struct {
	httpClient *http.Client
	region     string
	apiHost    string
}

// NewWOWAPIClient creates a new WOWAPIClient
func NewWOWAPIClient(client *http.Client, region string) *WOWAPIClient {
	return &WOWAPIClient{
		httpClient: client,
		region:     region,
		apiHost:    fmt.Sprintf(_apiHostFormat, region),
	}
}

// GetConnectedRealms gets all known connected realms in the clients region.
func (c *WOWAPIClient) GetConnectedRealms() (ConnectedRealms, error) {
	// no args needed
	apiArgs := url.Values{}
	u := c.urlFromQueryAndPath("/data/wow/connected-realm/index", apiArgs)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	addDynamicRegionNamespace(req, c.region)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	dc := json.NewDecoder(resp.Body)
	parsedResponse := connectedRealmIndexResponse{}
	err = dc.Decode(&parsedResponse)
	if err != nil {
		return nil, err
	}

	realms := make(ConnectedRealms, len(parsedResponse.ConnectedRealms))
	for _, realm := range parsedResponse.ConnectedRealms {
		// drop the query string
		url := strings.Split(realm.Href, "?")[0]
		urlFrags := strings.Split(url, "/")
		idStr := urlFrags[len(urlFrags)-1]
		fmt.Printf("found id %v\n", idStr)
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert ID string to int")
		}
		cr, err := c.getConnectedRealm(id)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch connected realm %v", id)
		}

		for _, r := range cr.Realms {
			realms[r] = cr
		}
	}

	return realms, nil
}

func (c *WOWAPIClient) getConnectedRealm(id int) (ConnectedRealm, error) {
	// no args needed
	apiArgs := url.Values{}
	u := c.urlFromQueryAndPath(fmt.Sprintf("/data/wow/connected-realm/%v", id), apiArgs)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return ConnectedRealm{}, err
	}
	addDynamicRegionNamespace(req, c.region)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ConnectedRealm{}, err
	}

	dc := json.NewDecoder(resp.Body)
	parsedResponse := connectedRealmResponse{}
	err = dc.Decode(&parsedResponse)
	if err != nil {
		return ConnectedRealm{}, err
	}

	cr := ConnectedRealm{
		ID: id,
	}
	cr.Realms = make([]string, 0, len(parsedResponse.Realms))
	for _, r := range parsedResponse.Realms {
		cr.Realms = append(cr.Realms, r.Name)
	}
	return cr, nil
}

func (c *WOWAPIClient) urlFromQueryAndPath(path string, values url.Values) *url.URL {
	// Force all output to en_US for now
	values.Add("locale", "en_US")

	return &url.URL{
		Scheme:   "https",
		Host:     c.apiHost,
		Path:     path,
		RawQuery: values.Encode(),
	}
}

// ConnectedRealm is the smallest set of information about a connected realm that is needed
type ConnectedRealm struct {
	Realms []string
	ID     int
}

func addDynamicRegionNamespace(req *http.Request, region string) {
	req.Header.Add("Battlenet-Namespace", fmt.Sprintf("dynamic-%s", region))
}

type connectedRealmIndexResponse struct {
	Links           map[string]link `json:"_links"`
	ConnectedRealms []link          `json:"connected_realms"`
}

type connectedRealmResponse struct {
	Links    map[string]link `json:"_links"`
	ID       int             `json:"id"`
	HasQueue bool            `json:"has_queue"`
	Realms   []realmResponse `json:"realms"`
}

type realmResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type link struct {
	Href string
}

type auctionsResponse struct {
}

// ConnectedRealms is a collection of all ConnectedRealm's which can be fetched by friendly name
// rather than ID.
type ConnectedRealms map[string]ConnectedRealm

// Get a realm from ConnectedRealms, returns an error if that friendly name doesn't exist
func (cr ConnectedRealms) Get(n string) (ConnectedRealm, error) {
	if r, ok := cr[strings.ToLower(n)]; ok {
		return r, nil
	}
	return ConnectedRealm{}, fmt.Errorf("unknown connected realm %v", n)
}
