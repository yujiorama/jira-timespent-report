package jira

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

const (
	maxWorkerSize             = 10
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
        collect worklog toggle`
)

var (
	AuthUser        string
	AuthToken       string
	BaseURL         string
	Query           string
	Fields          []string
	Filter          string
	FieldNames      string
	MaxResult       int
	ApiVersion      string
	TimeUnit        string
	HoursPerDay     int
	DaysPerMonth    int
	Worklog         bool
	TargetYearMonth string

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

	appHashInstance = appHash{
		mutex: &sync.Mutex{},
		memo:  map[string]interface{}{},
	}
)

func init() {
	flag.StringVar(&BaseURL, "url", "https://your-jira.atlassian.net", "jira url")
	flag.StringVar(&Query, "query", "status = Closed AND updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)", "jira query language expression")
	flag.StringVar(&Filter, "filter", "", "jira search filter id")
	flag.StringVar(&FieldNames, "fields", "summary,status,timespent,timeoriginalestimate,aggregatetimespent,aggregatetimeoriginalestimate", "fields of jira issue")
	flag.IntVar(&MaxResult, "maxresult", defaultMaxResult, "max result for pagination")
	flag.StringVar(&ApiVersion, "api", defaultJiraRestApiVersion, "number of API Version of Jira REST API")
	flag.StringVar(&TimeUnit, "unit", "dd", "time unit format string")
	flag.IntVar(&HoursPerDay, "hours", defaultHoursPerDay, "work hours per day")
	flag.IntVar(&DaysPerMonth, "days", defaultDaysPerMonth, "work days per month")
	flag.BoolVar(&Worklog, "worklog", false, "collect worklog toggle")
	flag.StringVar(&TargetYearMonth, "targetym", "", "target year month(yyyy-MM)")
	flag.Usage = func() {
		fmt.Println(usageText)
	}
}

func SetFlags() {
	flag.Parse()
	Fields = strings.Split(FieldNames, ",")
	if Worklog {
		Fields = []string{
			"started",
			"author.displayname",
			"author.emailaddress",
			"timespentseconds",
		}
	}

	AuthUser = os.Getenv("AUTH_USER")
	AuthToken = os.Getenv("AUTH_TOKEN")
	if len(AuthUser) == 0 || len(AuthToken) == 0 {
		panic("環境変数 AUTH_USER/AUTH_TOKEN が未定義")
	}
}

func Do() {
	log.Println("start")
	SetFlags()
	issues, worklogs, searchErrors := Search()
	for _, err := range searchErrors {
		log.Printf("%v\n", err)
	}

	reportErrors := Report(os.Stdout, issues, worklogs)
	if reportErrors != nil {
		for _, err := range reportErrors {
			log.Printf("%v\n", err)
		}
	}

	log.Println("end")
}

func Search() (IssueSearchResults, WorklogResults, []error) {

	issues, searchErrors := IssueSearch(MaxResult)
	if !Worklog {
		var nothing WorklogResults
		return issues, nothing, searchErrors
	}

	worklogs, worklogErrors := WorklogSearch(issues)
	for _, err := range worklogErrors {
		searchErrors = append(searchErrors, err)
	}

	return issues, worklogs, searchErrors
}

func Report(w io.Writer, issues IssueSearchResults, worklogs WorklogResults) []error {

	renderErrors := make([]error, 0, 2)

	if issues != nil {
		if err := issues.RenderCsv(w, Fields); err != nil {
			renderErrors = append(renderErrors, err)
		}
	}

	if worklogs != nil {
		if err := worklogs.RenderCsv(w, Fields); err != nil {
			renderErrors = append(renderErrors, err)
		}
	}

	return renderErrors
}

func IssueSearch(maxResult int) (IssueSearchResults, []error) {

	results := make(IssueSearchResults, 0, 10)
	searchErrors := make([]error, 0, 10)

	resultCh, errorCh := searchCh([]int{1}, maxResult)
	if err := <-errorCh; err != nil {
		searchErrors = append(searchErrors, err)
	}
	firstResult := <-resultCh

	if firstResult != nil {
		if firstResult.IsNotEmpty() {
			results = append(results, *firstResult)
		}

		resultCh, errorCh := searchCh(firstResult.RestPages(), firstResult.MaxResults)
		for err := range errorCh {
			searchErrors = append(searchErrors, err)
		}
		for result := range resultCh {
			results = append(results, *result)
		}
	}

	return results, searchErrors
}

func WorklogSearch(results IssueSearchResults) (WorklogResults, []error) {

	worklogResults := make(WorklogResults, 0, 10)
	searchErrors := make([]error, 0, 10)

	worklogCh, errorCh := worklogCh(results)
	for err := range errorCh {
		searchErrors = append(searchErrors, err)
	}

	for worklog := range worklogCh {
		worklogResults = append(worklogResults, *worklog)
	}

	return worklogResults, searchErrors
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
