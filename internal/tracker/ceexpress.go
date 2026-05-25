package tracker

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// In-memory cache for CE Express responses
var (
	cache     sync.Map // map[string]cacheEntry
	cacheTTL  = 60 * time.Second
)

type cacheEntry struct {
	result    *TrackingResult
	cachedAt  time.Time
}

// SetCacheTTL allows overriding the default cache duration.
func SetCacheTTL(seconds int) {
	if seconds > 0 {
		cacheTTL = time.Duration(seconds) * time.Second
	}
}

// getCached returns cached result if still fresh, otherwise nil.
func getCached(code string) *TrackingResult {
	v, ok := cache.Load(code)
	if !ok {
		return nil
	}
	entry := v.(cacheEntry)
	if time.Since(entry.cachedAt) > cacheTTL {
		cache.Delete(code)
		return nil
	}
	return entry.result
}

// setCached stores the result with current timestamp.
func setCached(code string, result *TrackingResult) {
	cache.Store(code, cacheEntry{
		result:   result,
		cachedAt: time.Now(),
	})
}

type ShipmentData struct {
	Success bool `json:"success"`
	Data    struct {
		ShipmentList []struct {
			Shipment struct {
				ShipmentStatus string `json:"shipmentStatus"`
				DestAddrAll    string `json:"destAddrAll"`
				DestAddress    string `json:"destAddress"`
				TotalWeight    float64 `json:"totalWeight"`
			} `json:"shipment"`
			FullEventList []struct {
				EventCode         string `json:"eventCode"`
				EventTime         string `json:"eventTime"`
				Status            string `json:"status"`
				TrackingEventDesc string `json:"trackingEventDesc"`
				EventShop         string `json:"eventShop"`
				OperatorName      string `json:"operatorName"`
				OperatorMobile    string `json:"operatorMobile"`
			} `json:"fullEventList"`
		} `json:"shipmentList"`
	} `json:"data"`
}

type TrackingResult struct {
	ShipmentStatus string
	LastEventCode  string
	LastEventTime  string
	Delivered      bool
	Events         []EventInfo
	CourierName    string
	CourierMobile  string
	Destination    string
	Weight         float64
}

type EventInfo struct {
	Time        string
	Description string
	Shop        string
}

var deliveredEventCodes = map[string]bool{
	"70":        true,
	"DELIVERED": true,
	"POD":       true,
}

var shipmentStatusLabels = map[string]string{
	"10": "Created",
	"20": "Pickup Assigned",
	"30": "Picked Up / In Transit",
	"40": "Out for Delivery",
	"50": "Delivery Attempted",
	"60": "Delivered",
	"70": "Exception",
	"80": "Returned",
}

func Fetch(code string) (*TrackingResult, error) {
	// Check cache first
	if cached := getCached(code); cached != nil {
		log.Printf("[CE_EXPRESS] ⚡ cache HIT code=%s (no API call)", code)
		return cached, nil
	}

	start := time.Now()
	ts := start.UnixMilli()
	url := fmt.Sprintf("https://cp.cambodianexpress.com/api/public/shipment/track?code=%s&_=%d", code, ts)

	log.Printf("[CE_EXPRESS] → GET track code=%s (cache miss)", code)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[CE_EXPRESS] ✗ request build error for %s: %v", code, err)
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[CE_EXPRESS] ✗ http error for %s: %v (took %s)", code, err, time.Since(start))
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[CE_EXPRESS] ✗ %s returned %d (took %s)", code, resp.StatusCode, time.Since(start))
		return nil, fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	var data ShipmentData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[CE_EXPRESS] ✗ %s decode error: %v", code, err)
		return nil, err
	}

	if !data.Success || len(data.Data.ShipmentList) == 0 {
		log.Printf("[CE_EXPRESS] ⚠ %s no data found (took %s)", code, time.Since(start))
		return nil, fmt.Errorf("no data found for code %s", code)
	}

	log.Printf("[CE_EXPRESS] ✓ %s status=%s (took %s)", code, data.Data.ShipmentList[0].Shipment.ShipmentStatus, time.Since(start))

	shipment := data.Data.ShipmentList[0].Shipment
	events := data.Data.ShipmentList[0].FullEventList

	result := &TrackingResult{
		ShipmentStatus: shipment.ShipmentStatus,
		Destination:    shipment.DestAddrAll,
		Weight:         shipment.TotalWeight,
		Events:         []EventInfo{},
	}

	if result.Destination == "" {
		result.Destination = shipment.DestAddress
	}

	if len(events) > 0 {
		result.LastEventCode = events[len(events)-1].EventCode
		result.LastEventTime = events[len(events)-1].EventTime
		result.Delivered = isDelivered(shipment.ShipmentStatus, events)

		for _, ev := range events {
			result.Events = append(result.Events, EventInfo{
				Time:        ev.EventTime,
				Description: ev.TrackingEventDesc,
				Shop:        ev.EventShop,
			})
		}

		if shipment.ShipmentStatus == "40" && !result.Delivered {
			if len(events) > 0 {
				result.CourierName = events[len(events)-1].OperatorName
				result.CourierMobile = events[len(events)-1].OperatorMobile
			}
		}
	}

	// Cache the successful response
	setCached(code, result)

	return result, nil
}

// InvalidateCache removes a code from the cache (useful when worker detects changes).
func InvalidateCache(code string) {
	cache.Delete(code)
}

func isDelivered(status string, events []struct {
	EventCode         string `json:"eventCode"`
	EventTime         string `json:"eventTime"`
	Status            string `json:"status"`
	TrackingEventDesc string `json:"trackingEventDesc"`
	EventShop         string `json:"eventShop"`
	OperatorName      string `json:"operatorName"`
	OperatorMobile    string `json:"operatorMobile"`
}) bool {
	if status == "60" {
		return true
	}

	for _, ev := range events {
		if deliveredEventCodes[ev.EventCode] {
			return true
		}
		desc := ev.Status
		if desc == "" {
			desc = ev.TrackingEventDesc
		}
		if containsButNotPickup(desc) && !containsPickupAssign(desc) {
			if ev.EventCode != "60" || containsPOD(desc) {
				return true
			}
		}
	}
	return false
}

func containsButNotPickup(s string) bool {
	return contains(s, "DELIVER")
}

func containsPickupAssign(s string) bool {
	return contains(s, "ASSIGN")
}

func containsPOD(s string) bool {
	return contains(s, "POD")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func GetStatusLabel(code string) string {
	if label, ok := shipmentStatusLabels[code]; ok {
		return label
	}
	return fmt.Sprintf("Status %s", code)
}
