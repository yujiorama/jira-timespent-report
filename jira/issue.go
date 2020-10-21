package jira

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
)

func (a Issues) Len() int {

	return len(a)
}

func (a Issues) Swap(i, j int) {

	a[i], a[j] = a[j], a[i]
}

func (a Issues) Less(i, j int) bool {

	return a[i].Key < a[j].Key
}

func (i *Issue) ToRecord(fields []string) []string {

	result := []string{i.Key}
	for _, x := range i.Fields.ToRecord(fields) {
		result = append(result, x)
	}
	return result
}

func (f *IssueField) ToRecord(fields []string) []string {

	var result []string

	st := reflect.ValueOf(*f)
	for _, fieldName := range fields {
		v := ""

		structFieldName := strings.ToUpper(fieldName[:1]) + strings.ToLower(fieldName[1:])
		if field := st.FieldByName(structFieldName); field.IsValid() {
			switch fieldName {
			case "timespent", "timeoriginalestimate", "aggregatetimespent", "aggregatetimeoriginalestimate":
				second := int(field.Int())
				v = fmt.Sprintf("%.2f", config.WithTimeUnit(second))
			case "status":
				v = f.Status.Name
			default:
				switch field.Kind() {
				case reflect.String:
					v = field.String()
				case reflect.Int:
					v = fmt.Sprintf("%d", field.Int())
				case reflect.Float32:
					v = fmt.Sprintf("%f", field.Float())
				}
			}
		}

		result = append(result, v)
	}

	return result
}

func (r *IssueSearchResult) IsNotEmpty() bool {

	return r.Total > 0 && len(r.Issues) > 0
}

func (r *IssueSearchResult) RestPages() []int {

	current := r.StartAt/r.MaxResults + 1
	next := current + 1
	last := r.Total/r.MaxResults + 1

	pages := make([]int, 0, 10)
	for page := next; page <= last; page++ {
		pages = append(pages, page)
	}
	return pages
}

func (results IssueSearchResults) RenderCsv(w io.Writer, fields []string) error {

	fieldLabels := []string{"キー"}
	for _, field := range fields {
		label := field
		if text, ok := defaultFieldText[label]; ok {
			label = text
		}
		fieldLabels = append(fieldLabels, label)
	}
	writer := csv.NewWriter(w)
	if err := writer.Write(fieldLabels); err != nil {
		return fmt.Errorf("writer.Write error: %v\nfieldLabels=[%v]\n", err, fieldLabels)
	}

	allIssues := make(Issues, 0, 10)
	for _, result := range results {
		for _, issue := range result.Issues {
			allIssues = append(allIssues, issue)
		}
	}
	sort.Sort(allIssues)

	for _, issue := range allIssues {
		record := issue.ToRecord(fields)
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("writer.Write error: %v\nrecord=[%v]\n", err, record)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("writer.Error error: %v\n", err)
	}

	return nil
}

func getFilterJql(filterID string) (string, bool) {

	cacheKey := fmt.Sprintf("getFilterJql_%s", filterID)
	if v, ok := cache.get(cacheKey); ok {
		log.Printf("cache hit: key=[%s], v=[%v]\n", cacheKey, v)
		return v.(string), true
	}

	filterURL, err := config.FilterURL(filterID)
	if err != nil {
		log.Printf("config.FilterURL error: %v\nfilterID=[%v]\n", err, filterID)
		return "", false
	}

	req, err := http.NewRequest("GET", filterURL.String(), nil)
	if err != nil {
		log.Printf("http.NewRequest error: %v\nfilterURL=[%v]\n", err, filterURL)
		return "", false
	}

	req.Header.Set("Authorization", fmt.Sprintf(config.basicAuthorization()))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("client.Do error: %v\nreq=[%v]\n", err, req)
		return "", false
	}
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("ioutil.ReadAll error: %v\nresp.Body=[%v]\n", err, resp.Body)
		return "", false
	}
	var result struct {
		Jql string `json:"jql"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		log.Printf("json.Unmarshal error: %v\nresponseBody=[%v]\n", err, responseBody)
		return "", false
	}

	cache.put(cacheKey, result.Jql)
	return result.Jql, true
}

func getSearchResult(requestBody []byte) (*IssueSearchResult, error) {

	cacheKey := fmt.Sprintf("getSearchResult_%s", string(requestBody))
	if v, ok := cache.get(cacheKey); ok {
		log.Printf("cache hit: key=[%s], v=[%v]\n", cacheKey, v)
		result := v.(IssueSearchResult)
		return &result, nil
	}

	searchURL, err := config.SearchURL()
	if err != nil {
		return nil, fmt.Errorf("config.SearchURL error: %v", err)
	}

	req, err := http.NewRequest("POST", searchURL.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %v\nsearchURL=[%v],requestBody=[%v]",
			err, searchURL, requestBody)
	}

	req.Header.Set("Authorization", config.basicAuthorization())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do error: %v\nreq=[%v]", err, req)
	}
	defer resp.Body.Close()

	var result IssueSearchResult
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll error: %v\nresp.Body=[%v]", err, resp.Body)
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("json.Unmarshal error: %v\nresponseBody=[%v]", err, responseBody)
	}

	cache.put(cacheKey, result)
	return &result, nil
}

func searchCh(pages []int, issuesPerPage int) (<-chan *IssueSearchResult, <-chan error) {

	resultCh := make(chan *IssueSearchResult, len(pages))
	defer close(resultCh)
	errorCh := make(chan error, len(pages))
	defer close(errorCh)

	if len(pages) == 0 {
		return resultCh, errorCh
	}

	workerSize := len(pages)
	if workerSize > maxWorkerSize {
		workerSize = maxWorkerSize
	}

	var wg sync.WaitGroup
	wg.Add(workerSize)
	startAtCh := make(chan int, len(pages))
	for n := 0; n < workerSize; n++ {
		go searchWorker(n, &wg, startAtCh, resultCh, errorCh)
	}

	for _, page := range pages {
		startAtCh <- (page - 1) * issuesPerPage
	}
	close(startAtCh)
	wg.Wait()

	return resultCh, errorCh
}

func searchWorker(n int, wg *sync.WaitGroup, startAtCh <-chan int, resultCh chan<- *IssueSearchResult, errorCh chan<- error) {

	defer wg.Done()
	for startAt := range startAtCh {
		result, err := search(startAt)
		if err != nil {
			errorCh <- fmt.Errorf("search error: %v\nn=[%v],startAt=[%v]", err, n, startAt)
		}

		if result != nil {
			resultCh <- result
		}
	}
}

func search(startAt int) (*IssueSearchResult, error) {

	searchRequest := map[string]interface{}{
		"fields":     config.fields(),
		"startAt":    startAt,
		"maxResults": config.MaxResult,
	}
	if len(config.Query) > 0 {
		searchRequest["jql"] = config.Query
	}
	if len(config.TargetYearMonth) > 0 {
		if dateCondition, ok := config.dateCondition(); ok {
			searchRequest["jql"] = composeJql(searchRequest["jql"].(string), dateCondition)
		}
	}
	if len(config.Filter) > 0 {
		if filterQuery, ok := getFilterJql(config.Filter); ok {
			searchRequest["jql"] = filterQuery
		}
	}

	log.Printf("search: startAt=[%v],query=[%v]\n", startAt, searchRequest["jql"])
	requestBody, err := json.Marshal(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal error: %v\nsearchRequest=[%v]", err, searchRequest)
	}
	result, err := getSearchResult(requestBody)
	if err != nil {
		return nil, fmt.Errorf("getSearchResult error: %v\nrequestBody=[%v]", err, string(requestBody))
	}

	if result.IsNotEmpty() {
		return result, nil
	}

	return nil, fmt.Errorf("empty result")
}

func composeJql(baseQuery string, condition string) string {
	if len(condition) == 0 {
		return baseQuery
	}

	if strings.Contains(strings.ToLower(baseQuery), "worklogdate") {
		return baseQuery
	}

	if strings.Contains(strings.ToLower(baseQuery), "updated") {
		return baseQuery
	}

	if strings.Contains(strings.ToLower(baseQuery), "order by") {
		i := strings.Index(strings.ToLower(baseQuery), "order by")
		return fmt.Sprintf("%s AND (%s) %s", baseQuery[0:i], condition, baseQuery[i:])
	}

	return fmt.Sprintf("%s AND (%s)", baseQuery, condition)
}
