package jira

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
)

func (a Worklogs) Len() int {

	return len(a)
}

func (a Worklogs) Swap(i, j int) {

	a[i], a[j] = a[j], a[i]
}

func (a Worklogs) Less(i, j int) bool {

	if a[i].Key == a[j].Key {
		return a[i].Started < a[j].Started
	}

	return a[i].Key < a[j].Key
}

func (w *WorklogField) ToRecord(fields []string) []string {

	result := []string{w.Key}

	st := reflect.ValueOf(*w)
	for _, fieldName := range fields {
		v := ""

		structFieldName := strings.ToUpper(fieldName[:1]) + strings.ToLower(fieldName[1:])
		if field := st.FieldByName(structFieldName); field.IsValid() {
			switch fieldName {
			case "timespentseconds":
				second := int(field.Int())
				v = fmt.Sprintf("%.2f", config.WithTimeUnit(second))
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

func (w *WorklogResult) IsNotEmpty() bool {

	return w.Total > 0 && len(w.Worklogs) > 0
}

func (results WorklogResults) RenderCsv(w io.Writer, fields []string) error {

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

	allWorklogs := make(Worklogs, 0, 10)
	for _, result := range results {
		allWorklogs = append(allWorklogs, result.Worklogs...)
	}
	sort.Sort(allWorklogs)

	for _, worklog := range allWorklogs {
		record := worklog.ToRecord(fields)
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

func getWorklogResult(key string, queryParams url.Values) (*WorklogResult, error) {

	worklogURL, err := config.WorklogURL(key, queryParams)
	if err != nil {
		return nil, fmt.Errorf("config.WorklogURL error: %v\nkey=[%v], queryParams=[%v]", err, key, queryParams)
	}

	req, err := http.NewRequest("GET", worklogURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %v\nworklogURL=[%v]", err, worklogURL)
	}

	req.Header.Set("Authorization", config.basicAuthorization())
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do error: %v\nreq=[%v]", err, req)
	}
	defer resp.Body.Close()

	var result WorklogResult
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

func worklogCh(results IssueSearchResults) (<-chan *WorklogResult, <-chan error) {

	bufferSize := 10
	if len(results) > 0 {
		bufferSize = results[0].Total
	}

	resultCh := make(chan *WorklogResult, bufferSize)
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

func worklogWorker(n int, keyCh <-chan string, resultCh chan<- *WorklogResult, errorCh chan<- error, wg *sync.WaitGroup) {

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

func worklog(key string) (*WorklogResult, error) {

	queryParams := url.Values{
		"startAt":      []string{"0"},
		"maxResults":   []string{"1048576"},
		"startedAfter": []string{config.StartedAfter()},
	}

	result, err := getWorklogResult(key, queryParams)
	if err != nil {
		return nil, fmt.Errorf("getWorklogResult error: %v\nkey=[%v], queryParams=[%v]",
			err, key, queryParams)
	}

	if result.IsNotEmpty() {
		return result, nil
	}

	return nil, fmt.Errorf("empty result")
}
