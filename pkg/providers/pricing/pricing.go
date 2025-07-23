/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zoom/karpenter-oci/pkg/operator/options"
	"github.com/zoom/karpenter-oci/pkg/providers/internalmodel"
	"io"
	"net/http"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter/pkg/utils/pretty"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	OcpuPerHour         = "OCPU Per Hour"
	GpuPerHour          = "GPU Per Hour"
	GigabytePerHour     = "Gigabyte Per Hour"
	NodePerHour         = "Node Per Hour"
	NVMeTerabytePerHour = "NVMe Terabyte Per Hour"
)

var specialTypeMap = map[string]string{
	"GPU2":       "GPU Standard - X7",
	"GPU3":       "GPU Standard - X7",
	"Standard1":  "Standard - X5",
	"Optimized3": "Standard - HPC - X7",
	"HPC":        "Standard - HPC - X7",
}

type Item struct {
	PartNumber      string `json:"partNumber"`
	DisplayName     string `json:"displayName"`
	MetricName      string `json:"metricName"`
	ServiceCategory string `json:"serviceCategory"`

	CurrencyCodeLocalizations []CurrencyCodeLocalization `json:"currencyCodeLocalizations"`
}

func (item Item) IsFree() bool {

	return strings.Contains(item.DisplayName, "Free")
}

func (item Item) IsGpu() bool {
	return strings.Contains(item.DisplayName, "GPU")
}

func (item Item) IsNvme() bool {
	return strings.Contains(item.DisplayName, "NVMe")
}

func (item Item) IsOcpuType() bool {

	return strings.Contains(item.DisplayName, "OCPU")
}

func (item Item) IsMemoryType() bool {
	return strings.Contains(item.DisplayName, "Memory")
}

func (item Item) IsNVMeType() bool {
	return strings.Contains(item.DisplayName, "NVMe")
}

func (item Item) IsHourlyCommit() bool {
	return strings.Contains(item.DisplayName, "Hourly Commit")
}

func (item Item) IsMonthCommit() bool {
	return strings.Contains(item.DisplayName, "1 Month Commit")
}

func (item Item) IsYearCommit() bool {
	return strings.Contains(item.DisplayName, "1 Year Commit")
}

func (item Item) Is3YearCommit() bool {
	return strings.Contains(item.DisplayName, "3 Year Commit")
}

func (item Item) PricePerUnit() float32 {

	if item.MetricName == NodePerHour {
		coreNum := parseCoreNumFromDisplayNum(item.DisplayName)
		return item.GetPrice(USD) / float32(coreNum)
	} else {

		return item.GetPrice(USD)
	}
}

func (item Item) GetPrice(code CurrencyCode) float32 {

	for _, local := range item.CurrencyCodeLocalizations {
		if local.CurrencyCode == code {
			return local.Prices[0].Value
		}
	}
	return 0
}

func (item Item) GetCpuNum() int {

	if !item.IsHourlyCommit() && !item.IsMonthCommit() && !item.IsYearCommit() {
		return 1
	}

	parts := strings.Split(item.DisplayName, "-")
	shape := parts[len(parts)-2]

	sParts := strings.Split(shape, ".")

	if len(sParts) <= 1 {
		return 1
	}

	if num, err := strconv.Atoi(strings.TrimSpace(sParts[len(sParts)-1])); err != nil {
		return 1
	} else {
		return num
	}

}

type CurrencyCodeLocalization struct {
	CurrencyCode CurrencyCode `json:"currencyCode"`
	Prices       []Price      `json:"prices"`
}

type Price struct {
	Model string  `json:"model"`
	Value float32 `json:"value"`
}

type PriceCatalog struct {
	Items []Item
}

// FindPriceItems retrieves matching PriceItems for the given shape.
func (catalog PriceCatalog) FindPriceItems(shape string) []Item {
	parsedShape, err := ParseShape(shape)
	if err != nil {
		return nil
	}

	// match special case
	if v, ok := specialTypeMap[parsedShape.ServiceType]; ok {
		parsedShape.ServiceType = v
	}

	candidates := findCandidate(catalog.Items, []string{parsedShape.ServiceCategory})

	// Further filter candidates based on display name
	var matchingItems []Item
	searchKey := parsedShape.ServiceType
	if len(parsedShape.CpuGpuType) > 0 {

		searchKey = fmt.Sprintf("%s - %s", searchKey, parsedShape.CpuGpuType)
	}
	for _, candidate := range candidates {
		if strings.Contains(candidate.DisplayName, searchKey) {

			if matched, err := regexp.MatchString(`\b`+searchKey+`\b`, candidate.DisplayName); matched && err == nil {

				matchingItems = append(matchingItems, candidate) // Add to matching items
			}
		}
	}

	// find from candidate serviceCategory
	if len(matchingItems) == 0 {
		candidates = findCandidate(catalog.Items, parsedShape.CandidateServiceCategory)
		for _, candidate := range candidates {
			if strings.Contains(candidate.DisplayName, searchKey) {
				matchingItems = append(matchingItems, candidate) // Add to matching items
			}
		}
	}

	return matchingItems
}

func findCandidate(items []Item, serviceCategories []string) []Item {
	// Search for candidates based on service category
	var candidates []Item
	for _, item := range items {
		for _, category := range serviceCategories {

			if item.ServiceCategory == category {
				candidates = append(candidates, item)
			}
		}
	}

	return candidates
}

// ParsedShape holds the parsed details of a shape.
type ParsedShape struct {
	ServiceCategory          string
	CandidateServiceCategory []string
	ServiceType              string
	CpuGpuType               string
	CpuGpuScale              string
}

// ParseShape Function to parse the shape string
func ParseShape(shape string) (ParsedShape, error) {
	parsed := ParsedShape{}
	items := strings.Split(shape, ".")
	if len(items) < 2 {
		return parsed, errors.New("invalid shape name")
	}

	// Determine service category
	if strings.Contains(items[1], "GPU") {
		parsed.ServiceCategory = "Compute - GPU"
	} else if items[0] == "BM" {
		parsed.ServiceCategory = "Compute - Bare Metal"
		parsed.CandidateServiceCategory = []string{"Compute - Virtual Machine", "Compute - VMware"}
	} else {
		parsed.ServiceCategory = "Compute - Virtual Machine"
		parsed.CandidateServiceCategory = []string{"Compute - VMware"}
	}

	// Determine service/shape type based on items[1]
	if parsed.ServiceCategory == "Compute - GPU" {
		re := regexp.MustCompile(`GPU(\d+)`)
		if re.MatchString(items[1]) {
			parsed.ServiceType = items[1]
		} else {
			parsed.ServiceType = "GPU"
		}
	} else {
		if strings.Contains(items[1], "DenseIO") {
			re := regexp.MustCompile(`DenseIO(\d+)`)
			if !re.MatchString(items[1]) {
				parsed.ServiceType = "Dense I/O"
			} else {
				parsed.ServiceType = items[1]
			}
		} else {
			// Check for "Standard" followed by any number
			re := regexp.MustCompile(`^Standard(\d*)`)
			hpcRe := regexp.MustCompile(`^HPC(\d*)`)
			if re.MatchString(items[1]) {
				parsed.ServiceType = re.FindString(items[1]) // Capture "Standard" and any following numbers
			} else if hpcRe.MatchString(items[1]) {
				parsed.ServiceType = "HPC"
			} else {
				parsed.ServiceType = items[1]
			}
		}

	}
	// Determine CPU/GPU type and scale
	if len(items) > 3 {
		parsed.CpuGpuType = items[2]
		if isNumeric(items[3]) {
			parsed.CpuGpuScale = items[3]
		} else if items[3] == "Flex" {
			parsed.CpuGpuScale = "Flexible"
		}
	}

	return parsed, nil
}

// Check if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

type Provider interface {
	Price(shape *internalmodel.WrapShape) float32
	UpdateOnDemandPricing(context.Context) error
}

type DefaultProvider struct {
	muOnDemand   sync.RWMutex
	cm           *pretty.ChangeMonitor
	priceCatalog *PriceCatalog
	endpoint     string
}

func NewDefaultProvider(ctx context.Context, endpoint string) *DefaultProvider {
	p := &DefaultProvider{
		endpoint: endpoint,
		cm:       pretty.NewChangeMonitor(),
	}
	// sets the pricing data from the static default state for the provider
	p.Reset(ctx)

	return p
}

func (p *DefaultProvider) Price(shape *internalmodel.WrapShape) float32 {
	p.muOnDemand.RLock()
	defer p.muOnDemand.RUnlock()
	price := Calculate(shape, p.priceCatalog)
	return price
}

func (p *DefaultProvider) UpdateOnDemandPricing(ctx context.Context) error {
	p.muOnDemand.Lock()
	defer p.muOnDemand.Unlock()
	if options.FromContext(ctx).UseLocalPriceList {
		return nil
	}
	catalog, err := p.Get()
	if err != nil {
		return fmt.Errorf("retreiving on-demand pricing data, %w", err)
	}
	p.priceCatalog = catalog
	if p.cm.HasChanged("instance-type-prices", p.priceCatalog) {
		log.FromContext(ctx).WithValues("instance-type-count", len(p.priceCatalog.Items)).V(1).Info("updated on-demand pricing")
	}
	return nil
}

func (p *DefaultProvider) Reset(ctx context.Context) {
	staticCatalog := &PriceCatalog{}
	err := json.Unmarshal([]byte(defaultPrice), &staticCatalog)
	if err != nil {
		log.FromContext(ctx).V(1).Error(err, "failed to unmarshal default static pricing data")
		return
	}

	p.priceCatalog = staticCatalog
}

func (p *DefaultProvider) Get() (*PriceCatalog, error) {

	fmt.Printf("%+v, sync price list\n", time.Now())
	cli := http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := cli.Get(p.endpoint)
	if err != nil || resp.StatusCode > 299 {
		//log.Log.Error(err, "failed to pull oci price list.\n")
		fmt.Printf("failed to pull price list, %+v", err)
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("failed to close http connection, %+v\n", err)
		}
	}(resp.Body)

	body, er := io.ReadAll(resp.Body)
	if er != nil {
		return nil, fmt.Errorf("failed to read price list api, %+v", err)
	}

	priceCatalog := &PriceCatalog{}
	er = json.Unmarshal(body, priceCatalog)
	if er != nil {
		//log.Log.Error(err, "failed to decode oci price list.\n ")
		return nil, fmt.Errorf("failed to decode oci price list, %v", er)
	}

	return priceCatalog, nil
}

func parseCoreNumFromDisplayNum(displayName string) int {

	// Step 1: Split by '-'
	parts := strings.Split(displayName, " - ")
	if len(parts) < 2 {
		return 1
	}

	// Step 2: Get the second-to-last element
	secondToLast := parts[len(parts)-2]

	// Step 3: Split by '.' and take the last part
	subParts := strings.Split(secondToLast, ".")
	if len(subParts) == 0 {
		return 1
	}

	// Step 4: Get the last part
	result, err := strconv.Atoi(subParts[len(subParts)-1])
	if err != nil {
		return 1
	}
	return result
}
