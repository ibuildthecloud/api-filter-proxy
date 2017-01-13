package manager

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/rancher/api-filter-proxy/model"
)

var (
	configFile         string
	CattleURL          string
	DefaultDestination string
	ConfigFields       ConfigFileFields
	//PathPreFilters is the map storing path -> prefilters[]
	// Why this varilable? is it needed, seems like duplicated from ConfigFileFields
	PathPreFilters map[string][]Filter
	//PathDestinations is the map storing path -> prefilters[]
	// Same here
	PathDestinations map[string]Destination
)

//Destination defines the properties of a Destination
type Destination struct {
	DestinationURL string   `json:"destinationURL"`
	Paths          []string `json:"paths"`
}

//ConfigFileFields stores filter config
type ConfigFileFields struct {
	Prefilters   []Filter
	Destinations []Destination
}

//SetEnv sets the parameters necessary
func SetEnv(c *cli.Context) {
	// This is not a good pattern to set everything to globals. In general this makes unit testing harder.
	// Instead set the value in appropriate structs.
	configFile = c.GlobalString("config")

	if configFile == "" {
		log.Fatal("Please specify path to the APIfilter config.json file")
		return
	}

	CattleURL = c.GlobalString("cattle-url")
	if len(CattleURL) == 0 {
		log.Fatalf("CATTLE_URL is not set")
	}

	DefaultDestination = c.GlobalString("default-destination")
	if len(DefaultDestination) == 0 {
		log.Infof("DEFAULT_DESTINATION is not set, will use CATTLE_URL as default")
		DefaultDestination = CattleURL
	}

	if configFile == "" {
		// As much as possible avoid nest code. Flatter code is easier to read. The simplest
		// approach to not nesting is usually an early exit.
		return nil
	}

	configContent, err := ioutil.ReadFile(configFile)
	if err != nil {
		// log.Fatal should be largely avoided and only do fatal in main()
		// This should instead return errors.Wrapf(err, "Error reading config.json file at path %v", configFile)
		// Please use the github.com/pkg/errors package
		// Also notice you are not printing the error here
		log.Fatalf("Error reading config.json file at path %v", configFile)
	}

	// There is no reason for an else because if will exit
	ConfigFields = ConfigFileFields{}
	err = json.Unmarshal(configContent, &ConfigFields)
	if err != nil {
		// Same comment about fatal, just return
		log.Fatalf("config.json data format invalid, error : %v\n", err)
	}

	PathPreFilters = make(map[string][]Filter)
	for _, filter := range ConfigFields.Prefilters {
		for _, path := range filter.Paths {
			PathPreFilters[path] = append(PathPreFilters[path], filter)
		}
	}

	// What is the purpose of Destination?  Why would the request go to a different destination?
	PathDestinations = make(map[string]Destination)
	for _, destination := range ConfigFields.Destinations {
		for _, path := range destination.Paths {
			PathDestinations[path] = destination
		}
	}
}

// I would change header type to http.Header
func ProcessPreFilters(path string, body map[string]interface{}, headers map[string][]string) (map[string]interface{}, map[string][]string, string, model.ProxyError) {
	prefilters := PathPreFilters[path]
	log.Debugf("START -- Processing pre filters for request path %v", path)
	inputBody := body
	inputHeaders := headers
	for _, filter := range prefilters {
		log.Debugf("-- Processing pre filter %v for request path %v --", filter, path)

		// more idomatic
		requestData := FilterData{
			Body:    inputBody,
			Headers: inputHeaders,
		}

		// I don't really think the error handling wholistically is correct.
		// If the proxy returns an error or != 200 response we should return 503
		// to the client.  So I really don't know if ProxyError is all that useful of
		// a type because this method basically works or doesn't, so just a normal
		// error response seems sufficient
		responseData, err := filter.processFilter(requestData)
		if err != nil {
			log.Errorf("Error %v processing the filter %v", err, filter)
			svcErr := model.ProxyError{
				// Why is Status a string?
				Status:  strconv.Itoa(http.StatusInternalServerError),
				Message: fmt.Sprintf("Error %v processing the filter %v", err, filter),
			}
			return inputBody, inputHeaders, "", svcErr
		}
		if responseData.Status == 200 {
			// emptiness is probably a safer check than nil
			if len(responseData.Body) != 0 {
				inputBody = responseData.Body
			}
			// emptiness is probably a safer check than nil
			if len(responseData.Headers) != 0 {
				inputHeaders = responseData.Headers
			}
		} else {
			//error
			log.Errorf("Error response %v - %v while processing the filter %v", responseData.Status, responseData.Body, filter)
			svcErr := model.ProxyError{
				Status:  strconv.Itoa(responseData.Status),
				Message: fmt.Sprintf("Error response while processing the filter %v", filter.Endpoint),
			}

			return inputBody, inputHeaders, "", svcErr
		}
	}

	//send the final body and headers to destination
	destination, ok := PathDestinations[path]
	destinationURL := destination.DestinationURL
	if !ok {
		destinationURL = DefaultDestination
	}
	log.Debugf("DONE -- Processing pre filters for request path %v, following to destination %v", path, destinationURL)

	return inputBody, inputHeaders, destinationURL, model.ProxyError{}
}
