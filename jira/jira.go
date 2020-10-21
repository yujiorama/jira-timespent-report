package jira

import (
	"flag"
	"fmt"
	"io"
)

func init() {
	flag.Usage = func() {
		fmt.Println(usageText)
	}
	flag.StringVar(&config.BaseURL, "url", "https://your-jira.atlassian.net", "jira url")
	flag.StringVar(&config.Query, "query", "status = Closed AND updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)", "jira query language expression")
	flag.StringVar(&config.Filter, "filter", "", "jira search filter id")
	flag.StringVar(&config.FieldNames, "fields", "summary,status,timespent,timeoriginalestimate,aggregatetimespent,aggregatetimeoriginalestimate", "fields of jira issue")
	flag.IntVar(&config.MaxResult, "maxresult", defaultMaxResult, "max result for pagination")
	flag.StringVar(&config.ApiVersion, "api", defaultJiraRestApiVersion, "number of API Version of Jira REST API")
	flag.StringVar(&config.TimeUnit, "unit", "dd", "time unit format string")
	flag.IntVar(&config.HoursPerDay, "hours", defaultHoursPerDay, "work hours per day")
	flag.IntVar(&config.DaysPerMonth, "days", defaultDaysPerMonth, "work days per month")
	flag.BoolVar(&config.Worklog, "worklog", false, "collect worklog toggle")
	flag.StringVar(&config.TargetYearMonth, "targetym", "", "target year month(yyyy-MM)")
}

func SetFlags() {
	flag.Parse()

	if err := config.checkAuthEnv(); err != nil {
		panic(err)
	}
}

func Search() (IssueSearchResults, WorklogResults, []error) {

	issues, searchErrors := IssueSearch(config.MaxResult)
	if !config.Worklog {
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
		if err := issues.RenderCsv(w, config.fields()); err != nil {
			renderErrors = append(renderErrors, err)
		}
	}

	if worklogs != nil {
		if err := worklogs.RenderCsv(w, config.fields()); err != nil {
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
