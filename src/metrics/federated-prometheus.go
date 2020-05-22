package metrics

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/kafkaesque-io/burnell/src/util"
)

var (
	cacheLock = sync.RWMutex{}
	cache     string

	interestedMetrics = map[string]bool{
		"pulsar_consumers_count":                          true,
		"pulsar_entry_size_count":                         true,
		"pulsar_entry_size_le_100_kb":                     true,
		"pulsar_entry_size_le_128":                        true,
		"pulsar_entry_size_le_16_kb":                      true,
		"pulsar_entry_size_le_1_kb":                       true,
		"pulsar_entry_size_le_1_mb":                       true,
		"pulsar_entry_size_le_2_kb":                       true,
		"pulsar_entry_size_le_4_kb":                       true,
		"pulsar_entry_size_le_512":                        true,
		"pulsar_entry_size_le_overflow":                   true,
		"pulsar_entry_size_sum":                           true,
		"pulsar_in_bytes_total":                           true,
		"pulsar_in_messages_total":                        true,
		"pulsar_msg_backlog":                              true,
		"pulsar_producers_count":                          true,
		"pulsar_rate_in":                                  true,
		"pulsar_rate_out":                                 true,
		"pulsar_storage_backlog_quota_limit":              true,
		"pulsar_storage_backlog_size":                     true,
		"pulsar_storage_offloaded_size":                   true,
		"pulsar_storage_size":                             true,
		"pulsar_storage_write_latency_count":              true,
		"pulsar_storage_write_latency_le_0_5":             true,
		"pulsar_storage_write_latency_le_1":               true,
		"pulsar_storage_write_latency_le_10":              true,
		"pulsar_storage_write_latency_le_100":             true,
		"pulsar_storage_write_latency_le_1000":            true,
		"pulsar_storage_write_latency_le_20":              true,
		"pulsar_storage_write_latency_le_200":             true,
		"pulsar_storage_write_latency_le_5":               true,
		"pulsar_storage_write_latency_le_50":              true,
		"pulsar_storage_write_latency_overflow":           true,
		"pulsar_storage_write_latency_sum":                true,
		"pulsar_subscription_back_log":                    true,
		"pulsar_subscription_blocked_on_unacked_messages": true,
		"pulsar_subscription_delayed":                     true,
		"pulsar_subscription_msg_rate_out":                true,
		"pulsar_subscription_msg_rate_redeliver":          true,
		"pulsar_subscription_msg_throughput_out":          true,
		"pulsar_subscription_unacked_messages":            true,
		"pulsar_subscriptions_count":                      true,
		"pulsar_throughput_in":                            true,
		"pulsar_throughput_out":                           true,
		"pulsar_topics_count":                             true,
	}
)

var logger = log.WithFields(log.Fields{"app": "burnell,federated-prom-scraper"})

// SetCache sets the federated prom cache
func SetCache(c string) {
	cacheLock.Lock()
	cache = c
	cacheLock.Unlock()
}

// GetCache gets the federated prom cache
func GetCache() string {
	cacheLock.RLock()
	defer cacheLock.RUnlock()
	return cache
}

// Init initializes
func Init() {

	url := util.Config.FederatedPromURL
	interval := time.Duration(util.GetEnvInt("ScrapeFederatedPromIntervalSeconds", 35)) * time.Second
	logger.Infof("Federated Prometheus URL %s at interval %v\n", url, interval)
	if url != "" {
		go func(promURL string) {
			Scrape(promURL)
			for {
				select {
				case <-time.Tick(interval):
					Scrape(promURL)
				}
			}
		}(url)
	}
}

// FilterFederatedMetrics collects the metrics the subject is allowed to access
func FilterFederatedMetrics(subject string) string {
	var rc string
	scanner := bufio.NewScanner(strings.NewReader(GetCache()))

	pattern := fmt.Sprintf(`.*,namespace="%s.*`, subject)
	typeDefPattern := fmt.Sprintf(`^# TYPE .*`)
	typeDef := ""
	for scanner.Scan() {
		text := scanner.Text()
		matched, err := regexp.MatchString(typeDefPattern, text)
		if matched && err == nil {
			typeDef = text
		} else {
			matched, err = regexp.MatchString(pattern, text)
			if matched && err == nil {
				if typeDef == "" {
					rc = fmt.Sprintf("%s%s\n", rc, text)
				} else {
					rc = fmt.Sprintf("%s%s\n%s\n", rc, typeDef, text)
					typeDef = ""
				}
			}
		}
	}
	return rc
}

// AllNamespaceMetrics returns all namespace metrics on the brokers
func AllNamespaceMetrics() string {
	return GetCache()
}

// Scrape scrapes the federated prometheus endpoint
func Scrape(url string) {
	client := &http.Client{Timeout: 600 * time.Second}

	// All prometheus jobs
	// req, err := http.NewRequest("GET", url+"/?match[]={__name__=~\"..*\"}", nil)
	req, err := http.NewRequest("GET", url+"/?match[]={job=~\"broker\"}", nil)
	if err != nil {
		logger.Errorf("url request error %s", err.Error())
		return
	}

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logger.Errorf("broker stats collection error %s", err.Error())
		return
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	c := string(bodyBytes)
	SetCache(c)

	logger.Infof("prometheus url %s resp status code %d cach size %d", url, resp.StatusCode, len(c))
}
