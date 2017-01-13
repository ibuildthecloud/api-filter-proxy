package manager

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	log "github.com/Sirupsen/logrus"
)

//Filter defines the properties of a pre/post API filter
type Filter struct {
	Endpoint    string   `json:"endpoint"`
	SecretToken string   `json:"secretToken"`
	Methods     []string `json:"methods"`
	Paths       []string `json:"paths"`
}

//FilterData defines the properties of a http Request/Response Body sent to/from a filter
type FilterData struct {
	// Is there a specific reason we don't send the method or path?  Those things seem quite useful.
	Headers map[string][]string    `json:"headers,omitempty"`
	Body    map[string]interface{} `json:"body,omitempty"`
	Status  int                    `json:"status,omitempty"`
}

func (filter *Filter) processFilter(input FilterData) (FilterData, error) {
	output := FilterData{}
	bodyContent, err := json.Marshal(input)
	if err != nil {
		return output, err
	}

	// Don't use +, but instead %s.  When debug is off you are wasting memory and CPU time
	// with +
	// Same applies to all log statements in this method
	log.Debugf("Request => " + string(bodyContent))

	// Don't create a new client.  You will leak FDs like a sieve.  Use a global instance.
	// You also need to ensure that you are setting the timeout properly
	client := &http.Client{}
	req, err := http.NewRequest("POST", filter.Endpoint, bytes.NewBuffer(bodyContent))
	if err != nil {
		return output, err
	}

	req.Header.Set("Content-Type", "application/json")
	// I believe this is unneeded, I think the NewRequest will do this
	req.Header.Set("Content-Length", string(len(bodyContent)))

	resp, err := client.Do(req)
	if err != nil {
		return output, err
	}
	log.Debugf("Response Status <= " + resp.Status)
	defer resp.Body.Close()

	// use json.Decoder here. Coping to bytes then unmarshal uses twice the memory
	byteContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return output, err
	}

	log.Debugf("Response <= " + string(byteContent))
	json.Unmarshal(byteContent, &output)
	output.Status = resp.StatusCode

	return output, nil
}
