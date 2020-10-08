package main

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
)

const (
	defaultMaxResult          = 50
	defaultHoursPerDay        = 8
	defaultDaysPerMonth       = 24
	defaultJiraRestApiVersion = "3"
	usageText                 = `Usage of jira-timespent-report:
  -api string
        number of API Version of Jira REST API (default "3")
  -days int
        work days per month (default 24)
  -fields string
        fields of jira issue (default "summary,status,timespent,timeoriginalestimate,aggregatetimespent,aggregatetimeoriginalestimate")
  -filter string
        jira search filter id
  -hours int
        work hours per day (default 8)
  -maxresult int
        max result for pagination (default 50)
  -query string
        jira query language expression (default "status = Closed AND updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)")
  -unit string
        time unit format string (default "dd")
  -url string
        jira url (default "https://your-jira.atlassian.net")`
)

var (
	authUser       string
	authToken      string
	jiraURL        string
	jiraQuery      string
	jiraFilter     string
	jiraFields     []string
	jiraMaxResult  int
	jiraApiVersion string
	timeUnit       string
	hoursPerDay    int
	daysPerMonth   int

	jiraIssueFieldLabel = map[string]string{
		"summary":                       "概要",
		"status":                        "ステータス",
		"timeoriginalestimate":          "初期見積もり",
		"timespent":                     "消費時間",
		"aggregatetimeoriginalestimate": "Σ初期見積もり",
		"aggregatetimespent":            "Σ消費時間",
	}
)

type status struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type issueField struct {
	Summary                       string `json:"summary"`
	Timespent                     int    `json:"timespent"`
	Timeoriginalestimate          int    `json:"timeoriginalestimate"`
	Aggregatetimespent            int    `json:"aggregatetimespent"`
	Aggregatetimeoriginalestimate int    `json:"aggregatetimeoriginalestimate"`
	Status                        status `json:"status,omitempty"`
}

type issue struct {
	Id     string     `json:"id"`
	Key    string     `json:"key"`
	Fields issueField `json:"fields"`
}

type issues []issue

type searchResult struct {
	StartAt    int    `json:"startAt"`
	Total      int    `json:"total"`
	Issues     issues `json:"issues"`
	MaxResults int    `json:"maxResults"`
}

func init() {
	flag.StringVar(&jiraURL, "url", "https://your-jira.atlassian.net", "jira url")
	flag.StringVar(&jiraQuery, "query", "status = Closed AND updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)", "jira query language expression")
	flag.StringVar(&jiraFilter, "filter", "", "jira search filter id")
	var fields string
	flag.StringVar(&fields, "fields", "summary,status,timespent,timeoriginalestimate,aggregatetimespent,aggregatetimeoriginalestimate", "fields of jira issue")
	jiraFields = strings.Split(fields, ",")
	flag.IntVar(&jiraMaxResult, "maxresult", defaultMaxResult, "max result for pagination")
	flag.StringVar(&jiraApiVersion, "api", defaultJiraRestApiVersion, "number of API Version of Jira REST API")
	flag.StringVar(&timeUnit, "unit", "dd", "time unit format string")
	flag.IntVar(&hoursPerDay, "hours", defaultHoursPerDay, "work hours per day")
	flag.IntVar(&daysPerMonth, "days", defaultDaysPerMonth, "work days per month")
}

func main() {
	flag.Usage = func() {
		fmt.Println(usageText)
	}
	flag.Parse()

	authUser = os.Getenv("AUTH_USER")
	authToken = os.Getenv("AUTH_TOKEN")
	if len(authUser) == 0 || len(authToken) == 0 {
		panic("環境変数 AUTH_USER/AUTH_TOKEN が未定義")
	}

	log.Println("start")

	results := make([]searchResult, 0, 10)
	firstResult := <-searchCh(1, 1)
	if firstResult != nil {
		if firstResult.isNotEmpty() {
			results = append(results, *firstResult)
		}

		if firstResult.hasNextPage() {
			for result := range searchCh(firstResult.nextPage(), firstResult.lastPage()) {
				results = append(results, *result)
			}
		}
	}

	renderCsv(results)

	log.Println("end")
}

func (i *issue) ToRecord(fields []string) []string {

	result := []string{i.Key}
	for _, x := range i.Fields.ToRecord(fields) {
		result = append(result, x)
	}
	return result
}

func (f *issueField) ToRecord(fields []string) []string {

	var result []string

	st := reflect.ValueOf(*f)
	for _, fieldName := range fields {
		v := ""

		structFieldName := strings.ToUpper(fieldName[:1]) + strings.ToLower(fieldName[1:])
		if field := st.FieldByName(structFieldName); field.IsValid() {
			switch fieldName {
			case "timespent", "timeoriginalestimate", "aggregatetimespent", "aggregatetimeoriginalestimate":
				second := int(field.Int())
				var t float32
				switch strings.ToLower(timeUnit) {
				case "h", "hh":
					t = float32(second) / float32(60*60)
				case "d", "dd":
					t = float32(second) / float32(60*60*hoursPerDay)
				case "m", "mm":
					t = float32(second) / float32(60*60*hoursPerDay*daysPerMonth)
				}
				v = fmt.Sprintf("%.2f", t)

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

func (r *searchResult) isNotEmpty() bool {

	return r.Total > 0 && len(r.Issues) > 0
}

func (r *searchResult) hasNextPage() bool {

	return r.currentPage() < r.lastPage()
}

func (r *searchResult) currentPage() int {

	return r.StartAt/r.MaxResults + 1
}

func (r *searchResult) lastPage() int {

	return r.Total/r.MaxResults + 1
}

func (r *searchResult) nextPage() int {

	return r.currentPage() + 1
}

func (a issues) Len() int {

	return len(a)
}

func (a issues) Swap(i, j int) {

	a[i], a[j] = a[j], a[i]
}

func (a issues) Less(i, j int) bool {

	return a[i].Key < a[j].Key
}

func getFilterJql(baseURL url.URL) (string, bool) {

	filterURL := baseURL
	filterURL.Path = fmt.Sprintf("/rest/api/%s/filter/%s", jiraApiVersion, jiraFilter)
	req, err := http.NewRequest("GET", filterURL.String(), nil)
	if err != nil {
		log.Printf("http.NewRequest error: %v\nfilterURL=[%v]\n", err, filterURL)
		return "", false
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(authUser+":"+authToken))))
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

	return result.Jql, true
}

func getSearchResult(baseURL url.URL, searchRequest map[string]interface{}) (*searchResult, error) {

	requestBody, err := json.Marshal(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal error: %v\nsearchRequest=[%v]", err, searchRequest)
	}
	searchURL := baseURL
	searchURL.Path = fmt.Sprintf("/rest/api/%s/search", jiraApiVersion)
	req, err := http.NewRequest("POST", searchURL.String(), bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %v\nsearchURL=[%v],requestBody=[%v]",
			err, searchURL, requestBody)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(authUser+":"+authToken))))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do error: %v\nreq=[%v]", err, req)
	}
	defer resp.Body.Close()

	var result searchResult
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll error: %v\nresp.Body=[%v]", err, resp.Body)
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("json.Unmarshal error: %v\nresponseBody=[%v]", err, responseBody)
	}

	return &result, nil
}

func searchCh(pageFromInclusive int, pageToInclusive int) <-chan *searchResult {

	results := make(chan *searchResult, 10)
	defer close(results)

	for page := pageFromInclusive; page <= pageToInclusive; page++ {

		startAt := (page - 1) * jiraMaxResult
		result, err := search(startAt)
		if err != nil {
			log.Printf("search error: %v\nstartAt=[%v]\n", err, startAt)
			continue
		}
		results <- result
	}

	return results
}

func search(startAt int) (*searchResult, error) {

	baseURL, err := url.Parse(jiraURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse error: %v\njiraURL=[%v]", err, jiraURL)
	}

	searchRequest := map[string]interface{}{
		"fields":     jiraFields,
		"startAt":    startAt,
		"maxResults": jiraMaxResult,
	}
	if len(jiraQuery) > 0 {
		searchRequest["jql"] = jiraQuery
	}
	if len(jiraFilter) > 0 {
		if filterQuery, ok := getFilterJql(*baseURL); ok {
			searchRequest["jql"] = filterQuery
		}
	}

	result, err := getSearchResult(*baseURL, searchRequest)
	if err != nil {
		return nil, fmt.Errorf("getSearchResult error: %v\nbaseURL=[%v], searchRequest=[%v]",
			err, baseURL, searchResult{})
	}

	if !result.isNotEmpty() {
		return nil, fmt.Errorf("empty result")
	}

	return result, nil
}

func renderCsv(results []searchResult) {

	fieldLabels := []string{"キー"}
	for _, field := range jiraFields {
		label := field
		if text, ok := jiraIssueFieldLabel[label]; ok {
			label = text
		}
		fieldLabels = append(fieldLabels, label)
	}
	writer := csv.NewWriter(os.Stdout)
	if err := writer.Write(fieldLabels); err != nil {
		log.Fatalf("writer.Write error: %v\nfieldLabels=[%v]\n", err, fieldLabels)
	}

	allIssues := make(issues, 0, 10)
	for _, result := range results {
		for _, issue := range result.Issues {
			allIssues = append(allIssues, issue)
		}
	}
	sort.Sort(allIssues)

	for _, issue := range allIssues {
		record := issue.ToRecord(jiraFields)
		if err := writer.Write(record); err != nil {
			log.Fatalf("writer.Write error: %v\nrecord=[%v]\n", err, record)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("writer.Error error: %v\n", err)
	}
}
