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
	"sync"
	"time"
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
  -targetym string
        target year month(yyyy-MM)
  -unit string
        time unit format string (default "dd")
  -url string
        jira url (default "https://your-jira.atlassian.net")
  -worklog
        collect worklog toggle
`
)

const (
	maxWorkerSize = 10
)

var (
	authUser        string
	authToken       string
	jiraURL         string
	jiraQuery       string
	jiraFilter      string
	jiraFields      []string
	jiraFieldNames  string
	jiraMaxResult   int
	jiraApiVersion  string
	jiraWorklog     bool
	timeUnit        string
	hoursPerDay     int
	daysPerMonth    int
	targetYearMonth string

	defaultFieldText = map[string]string{
		"summary":                       "概要",
		"status":                        "ステータス",
		"timeoriginalestimate":          "初期見積もり",
		"timespent":                     "消費時間",
		"aggregatetimeoriginalestimate": "Σ初期見積もり",
		"aggregatetimespent":            "Σ消費時間",
		"started":                       "開始日時",
		"author.displayname":            "表示名",
		"author.emailaddress":           "メールアドレス",
		"timespentseconds":              "消費時間",
	}
)

var (
	appHashInstance = appHash{
		mutex: &sync.Mutex{},
		memo:  map[string]interface{}{},
	}
)

type appHash struct {
	mutex *sync.Mutex
	memo  map[string]interface{}
}

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
	MaxResults int    `json:"maxResults"`
	Issues     issues `json:"issues"`
}

type searchResults []searchResult

type worklogField struct {
	Key    string
	Author struct {
		Displayname  string `json:"displayName"`
		Emailaddress string `json:"emailAddress"`
	} `json:"author"`
	Started          string `json:"started"`
	Timespentseconds int    `json:"timespentSeconds"`
}

type worklogs []worklogField

type worklogResult struct {
	StartAt    int      `json:"startAt"`
	Total      int      `json:"total"`
	MaxResults int      `json:"maxResults"`
	Worklogs   worklogs `json:"worklogs"`
}

type worklogResults []worklogResult

func init() {
	flag.StringVar(&jiraURL, "url", "https://your-jira.atlassian.net", "jira url")
	flag.StringVar(&jiraQuery, "query", "status = Closed AND updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)", "jira query language expression")
	flag.StringVar(&jiraFilter, "filter", "", "jira search filter id")
	flag.StringVar(&jiraFieldNames, "fields", "summary,status,timespent,timeoriginalestimate,aggregatetimespent,aggregatetimeoriginalestimate", "fields of jira issue")
	flag.IntVar(&jiraMaxResult, "maxresult", defaultMaxResult, "max result for pagination")
	flag.StringVar(&jiraApiVersion, "api", defaultJiraRestApiVersion, "number of API Version of Jira REST API")
	flag.StringVar(&timeUnit, "unit", "dd", "time unit format string")
	flag.IntVar(&hoursPerDay, "hours", defaultHoursPerDay, "work hours per day")
	flag.IntVar(&daysPerMonth, "days", defaultDaysPerMonth, "work days per month")
	flag.BoolVar(&jiraWorklog, "worklog", false, "collect worklog toggle")
	flag.StringVar(&targetYearMonth, "targetym", "", "target year month(yyyy-MM)")
}

func main() {
	flag.Parse()
	flag.Usage = func() {
		fmt.Println(usageText)
	}
	jiraFields = strings.Split(jiraFieldNames, ",")
	if jiraWorklog {
		jiraFields = []string{
			"started",
			"author.displayname",
			"author.emailaddress",
			"timespentseconds",
		}
	}

	authUser = os.Getenv("AUTH_USER")
	authToken = os.Getenv("AUTH_TOKEN")
	if len(authUser) == 0 || len(authToken) == 0 {
		panic("環境変数 AUTH_USER/AUTH_TOKEN が未定義")
	}

	log.Println("start")

	resultCh, errorCh := searchCh([]int{1}, jiraMaxResult)
	if err := <-errorCh; err != nil {
		log.Printf("%v\n", err)
	}
	firstResult := <-resultCh

	results := make(searchResults, 0, 10)
	if firstResult != nil {
		if firstResult.isNotEmpty() {
			results = append(results, *firstResult)
		}

		resultCh, errorCh := searchCh(firstResult.restPages(), firstResult.MaxResults)
		for err := range errorCh {
			log.Printf("%v\n", err)
		}
		for result := range resultCh {
			results = append(results, *result)
		}
	}

	if jiraWorklog {
		worklogCh, errorCh := worklogCh(results)
		for err := range errorCh {
			log.Printf("%v\n", err)
		}
		worklogResults := make(worklogResults, 0, 10)
		for worklog := range worklogCh {
			worklogResults = append(worklogResults, *worklog)
		}
		worklogResults.renderCsv()
	} else {
		results.renderCsv()
	}

	log.Println("end")
}

func (r *searchResult) isNotEmpty() bool {

	return r.Total > 0 && len(r.Issues) > 0
}

func (r *searchResult) restPages() []int {

	current := r.StartAt/r.MaxResults + 1
	next := current + 1
	last := r.Total/r.MaxResults + 1

	pages := make([]int, 0, 10)
	for page := next; page <= last; page++ {
		pages = append(pages, page)
	}
	return pages
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

func (c appHash) put(k string, v interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.memo[k] = v
}

func (c appHash) get(k string) (interface{}, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	v, ok := c.memo[k]
	return v, ok
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

func (results searchResults) renderCsv() {

	fieldLabels := []string{"キー"}
	for _, field := range jiraFields {
		label := field
		if text, ok := defaultFieldText[label]; ok {
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

func (w *worklogResult) isNotEmpty() bool {

	return w.Total > 0 && len(w.Worklogs) > 0
}

func (a worklogs) Len() int {

	return len(a)
}

func (a worklogs) Swap(i, j int) {

	a[i], a[j] = a[j], a[i]
}

func (a worklogs) Less(i, j int) bool {

	if a[i].Key == a[j].Key {
		return a[i].Started < a[j].Started
	}

	return a[i].Key < a[j].Key
}
func (w *worklogField) ToRecord(fields []string) []string {

	result := []string{w.Key}

	st := reflect.ValueOf(*w)
	for _, fieldName := range fields {
		v := ""

		structFieldName := strings.ToUpper(fieldName[:1]) + strings.ToLower(fieldName[1:])
		if field := st.FieldByName(structFieldName); field.IsValid() {
			switch fieldName {
			case "timespentseconds":
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

		if strings.HasPrefix(fieldName, "author.") {
			if "author.displayname" == fieldName {
				v = w.Author.Displayname
			}

			if "author.emailaddress" == fieldName {
				v = w.Author.Emailaddress
			}
		}

		result = append(result, v)
	}

	return result
}

func (results worklogResults) renderCsv() {

	fieldLabels := []string{"キー"}
	for _, field := range jiraFields {
		label := field
		if text, ok := defaultFieldText[label]; ok {
			label = text
		}
		fieldLabels = append(fieldLabels, label)
	}
	log.Println(jiraFields)
	writer := csv.NewWriter(os.Stdout)
	if err := writer.Write(fieldLabels); err != nil {
		log.Fatalf("writer.Write error: %v\nfieldLabels=[%v]\n", err, fieldLabels)
	}

	allWorklogs := make(worklogs, 0, 10)
	for _, result := range results {
		for _, worklog := range result.Worklogs {
			allWorklogs = append(allWorklogs, worklog)
		}
	}
	sort.Sort(allWorklogs)

	for _, worklog := range allWorklogs {
		record := worklog.ToRecord(jiraFields)
		if err := writer.Write(record); err != nil {
			log.Fatalf("writer.Write error: %v\nrecord=[%v]\n", err, record)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("writer.Error error: %v\n", err)
	}
}

func getFilterJql(baseURL url.URL, filterID string) (string, bool) {

	cacheKey := fmt.Sprintf("getFilterJql_%s", filterID)
	if v, ok := appHashInstance.get(cacheKey); ok {
		log.Printf("cache hit: key=[%s], v=[%v]\n", cacheKey, v)
		return v.(string), true
	}

	filterURL := baseURL
	filterURL.Path = fmt.Sprintf("/rest/api/%s/filter/%s", jiraApiVersion, filterID)
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

	appHashInstance.put(cacheKey, result.Jql)
	return result.Jql, true
}

func getSearchResult(baseURL url.URL, requestBody []byte) (*searchResult, error) {

	cacheKey := fmt.Sprintf("getSearchResult_%s", string(requestBody))
	if v, ok := appHashInstance.get(cacheKey); ok {
		log.Printf("cache hit: key=[%s], v=[%v]\n", cacheKey, v)
		result := v.(searchResult)
		return &result, nil
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

	appHashInstance.put(cacheKey, result)
	return &result, nil
}

func getWorklogResult(baseURL url.URL, key string, queryParams url.Values) (*worklogResult, error) {

	worklogURL := baseURL
	worklogURL.Path = fmt.Sprintf("/rest/api/%s/issue/%s/worklog", jiraApiVersion, key)
	worklogURL.RawQuery = queryParams.Encode()
	req, err := http.NewRequest("GET", worklogURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %v\nworklogURL=[%v]", err, worklogURL)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(authUser+":"+authToken))))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do error: %v\nreq=[%v]", err, req)
	}
	defer resp.Body.Close()

	var result worklogResult
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll error: %v\nresp.Body=[%v]", err, resp.Body)
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return nil, fmt.Errorf("json.Unmarshal error: %v\nresponseBody=[%v]", err, responseBody)
	}

	for i := range result.Worklogs {
		result.Worklogs[i].Key = key
	}

	return &result, nil
}

func searchCh(pages []int, issuesPerPage int) (<-chan *searchResult, <-chan error) {

	resultCh := make(chan *searchResult, len(pages))
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
		go searchWorker(n, startAtCh, resultCh, errorCh, &wg)
	}

	for _, page := range pages {
		startAtCh <- (page - 1) * issuesPerPage
	}
	close(startAtCh)
	wg.Wait()

	return resultCh, errorCh
}

func searchWorker(n int, startAtCh <-chan int, resultCh chan<- *searchResult, errorCh chan<- error, wg *sync.WaitGroup) {

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
	if len(targetYearMonth) > 0 {
		if dateCondition, ok := dateCondition(targetYearMonth, jiraWorklog); ok {
			searchRequest["jql"] = composeJql(searchRequest["jql"].(string), dateCondition)
		}
	}
	if len(jiraFilter) > 0 {
		if filterQuery, ok := getFilterJql(*baseURL, jiraFilter); ok {
			searchRequest["jql"] = filterQuery
		}
	}

	log.Printf("search: startAt=[%v],query=[%v]\n", startAt, searchRequest["jql"])
	requestBody, err := json.Marshal(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal error: %v\nsearchRequest=[%v]", err, searchRequest)
	}
	result, err := getSearchResult(*baseURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("getSearchResult error: %v\nbaseURL=[%v], requestBody=[%v]",
			err, baseURL, string(requestBody))
	}

	if result.isNotEmpty() {
		return result, nil
	}

	return nil, fmt.Errorf("empty result")
}

func dateCondition(yearMonth string, worklog bool) (string, bool) {

	t, err := time.Parse("2006-01-02", yearMonth+"-01")
	if err != nil {
		return "", false
	}

	offset := int(t.Month() - time.Now().Month())

	if worklog {
		return fmt.Sprintf("worklogDate >= startOfMonth(%d) AND worklogDate <= endOfMonth(%d)", offset, offset), true
	}

	return fmt.Sprintf("updated >= startOfMonth(%d) AND updated <= endOfMonth(%d)", offset, offset), true
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

func worklogCh(results searchResults) (<-chan *worklogResult, <-chan error) {

	bufferSize := 10
	if len(results) > 0 {
		bufferSize = results[0].Total
	}

	resultCh := make(chan *worklogResult, bufferSize)
	defer close(resultCh)
	errorCh := make(chan error, bufferSize)
	defer close(errorCh)

	if len(results) == 0 {
		return resultCh, errorCh
	}

	workerSize := len(results)
	if workerSize > maxWorkerSize {
		workerSize = maxWorkerSize
	}

	var wg sync.WaitGroup
	wg.Add(workerSize)
	keyCh := make(chan string, bufferSize)
	for n := 0; n < workerSize; n++ {
		go worklogWorker(n, keyCh, resultCh, errorCh, &wg)
	}

	for _, searchResult := range results {
		for _, issue := range searchResult.Issues {
			keyCh <- issue.Key
		}
	}
	close(keyCh)
	wg.Wait()

	return resultCh, errorCh
}

func worklogWorker(n int, keyCh <-chan string, resultCh chan<- *worklogResult, errorCh chan<- error, wg *sync.WaitGroup) {

	defer wg.Done()
	for key := range keyCh {
		result, err := worklog(key)
		if err != nil {
			errorCh <- fmt.Errorf("worklog error: %v\nn=[%v],key=[%v]", err, n, key)
		}

		if result != nil {
			resultCh <- result
		}
	}
}

func worklog(key string) (*worklogResult, error) {

	baseURL, err := url.Parse(jiraURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse error: %v\njiraURL=[%v]", err, jiraURL)
	}

	queryParams := url.Values{
		"startAt":    []string{"0"},
		"maxResults": []string{"1048576"},
		"startedAfter": []string{func() string {
			t := time.Now().AddDate(0, -1, 0)
			if len(targetYearMonth) > 0 {
				if v, err := time.Parse("2006-01-02", targetYearMonth+"-01"); err == nil {
					t = v
				}
			}
			return startOfMonthEpocMillis(t)
		}()},
	}

	result, err := getWorklogResult(*baseURL, key, queryParams)
	if err != nil {
		return nil, fmt.Errorf("getWorklogResult error: %v\nbaseURL=[%v], key=[%v], queryParams=[%v]",
			err, baseURL, key, queryParams)
	}

	if result.isNotEmpty() {
		return result, nil
	}

	return nil, fmt.Errorf("empty result")
}

func startOfMonthEpocMillis(t time.Time) string {
	startOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	return fmt.Sprintf("%d", startOfMonth.UnixNano()/int64(time.Millisecond))
}
