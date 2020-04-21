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

// GetItem gets an item from the wow API with the given ID.
func (c *WOWAPIClient) GetItem(id int) (Item, error) {
	// no url args needed
	resp := itemResponse{}
	if err := c.callAPI(fmt.Sprintf("/data/wow/item/%v", id), url.Values{}, &resp); err != nil {
		return Item{}, err
	}

	return Item{
		ID:             resp.ID,
		Name:           resp.Name,
		ItemClass:      resp.ItemClass.Name,
		ItemClassID:    resp.ItemClass.ID,
		ItemSubclass:   resp.ItemSubclass.Name,
		ItemSubclassID: resp.ItemSubclass.ID,
	}, nil
}

// GetAuctions gets all auctions from the given connected realm ID
func (c *WOWAPIClient) GetAuctions(realmID int) ([]Auction, error) {
	// no url args needed
	resp := auctionsResponse{}
	if err := c.callAPI(fmt.Sprintf("/data/wow/connected-realm/%v/auctions", realmID), url.Values{}, &resp); err != nil {
		return nil, err
	}

	auctions := make([]Auction, 0, len(resp.Auctions))
	for _, a := range resp.Auctions {
		auction := Auction{
			RealmID:   realmID,
			ID:        a.ID,
			ItemID:    a.Item.ID,
			Quantity:  a.Quantity,
			UnitPrice: a.UnitPrice,
			Buyout:    a.Buyout,
			Bid:       a.Bid,
			TimeLeft:  a.TimeLeft,
		}
		if auction.Quantity == 0 {
			return nil, fmt.Errorf("auction id %v of %v has a quantity of 0", auction.ID, auction.ItemID)
		}
		if auction.Buyout == 0 && auction.UnitPrice == 0 {
			return nil, fmt.Errorf("auction id %v of %v has a buyout of 0 and a unitprice of 0", auction.ID, auction.ItemID)
		}

		auctions = append(auctions, auction)
	}

	return auctions, nil
}

func (c *WOWAPIClient) callAPI(path string, queryArgs url.Values, responseTarget interface{}) error {
	u := c.urlFromQueryAndPath(path, queryArgs)

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return errors.Wrapf(err, "failed to call %v", u.String())
	}
	addDynamicRegionNamespace(req, c.region)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to call %v", u.String())
	}

	dc := json.NewDecoder(resp.Body)
	err = dc.Decode(responseTarget)
	if err != nil {
		return errors.Wrapf(err, "failed to call %v", u.String())
	}

	return nil
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

type itemResponse struct {
	Links        map[string]link `json:"_links"`
	ID           int             `json:"id"`
	Name         string          `json:"name"`
	ItemClass    itemClass
	ItemSubclass itemClass
}

type itemClass struct {
	Key  map[string]link `json:"key"`
	Name string          `json:"name"`
	ID   int             `json:"id"`
}

// Item is a minimal representation of an ingame item.
type Item struct {
	ID             int
	Name           string
	ItemClass      string
	ItemClassID    int
	ItemSubclass   string
	ItemSubclassID int
}

type auctionsResponse struct {
	Links          map[string]link   `json:"_links"`
	ConnectedRealm link              `json:"connected_realm"`
	Auctions       []auctionResponse `json:"auctions"`
}

type auctionResponse struct {
	ID   int `json:"id"`
	Item struct {
		ID int `json:"id"`
	} `json:"item"`
	Quantity  int      `json:"quantity"`
	UnitPrice int      `json:"unit_price,omitempty"`
	Buyout    int      `json:"buyout,omitempty"`
	Bid       int      `json:"bid"`
	TimeLeft  TimeLeft `json:"time_left"`
}

// Auction represents a single auction within a single region. Currently this does not support
// Items with "bonuses" or "modifiers" such as sockets, ilvl upgrades (warforge), or extra secondaries
// such as Indestructible or Speed. An Auction will either have a Buyout price or a Bid price, or a UnitPrice. If Buyout
// or Bid >0 it should be used.
type Auction struct {
	RealmID   int
	ID        int
	ItemID    int
	Quantity  int
	UnitPrice int
	Buyout    int
	Bid       int
	TimeLeft  TimeLeft
}

// TimeLeft represents how much time is left in an auction. The API makes this deliberately imprecise.
type TimeLeft string

// UnmarshalJSON unmarshals a TimeLeft from a json field.
func (tl *TimeLeft) UnmarshalJSON(b []byte) error {
	toMatch := strings.Trim(string(b), `"`)
	switch TimeLeft(toMatch) {
	case TimeLeftVeryLong:
		*tl = TimeLeftVeryLong
	case TimeLeftLong:
		*tl = TimeLeftLong
	case TimeLeftMedium:
		*tl = TimeLeftMedium
	case TimeLeftShort:
		*tl = TimeLeftShort
	default:
		return fmt.Errorf("cannot unmarshal %q as TimeLeft", toMatch)
	}

	return nil
}

const (
	// TimeLeftShort means there is less than 2 hours left on the auction.
	TimeLeftShort TimeLeft = "SHORT"
	// TimeLeftMedium means there is 2-12 hours left on the auction.
	TimeLeftMedium = "MEDIUM"
	// TimeLeftLong means there is 12-24 hours left on the auction.
	TimeLeftLong = "LONG"
	// TimeLeftVeryLong means there is more than 24 hours left on the auction.
	TimeLeftVeryLong = "VERY_LONG"
)
