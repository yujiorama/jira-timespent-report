package jira

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
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
				var t float32
				switch strings.ToLower(TimeUnit) {
				case "h", "hh":
					t = float32(second) / float32(60*60)
				case "d", "dd":
					t = float32(second) / float32(60*60*HoursPerDay)
				case "m", "mm":
					t = float32(second) / float32(60*60*HoursPerDay*DaysPerMonth)
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

func (w *WorklogResult) IsNotEmpty() bool {

	return w.Total > 0 && len(w.Worklogs) > 0
}

func (results WorklogResults) RenderCsv(fields []string) {

	fieldLabels := []string{"キー"}
	for _, field := range fields {
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

	allWorklogs := make(Worklogs, 0, 10)
	for _, result := range results {
		for _, worklog := range result.Worklogs {
			allWorklogs = append(allWorklogs, worklog)
		}
	}
	sort.Sort(allWorklogs)

	for _, worklog := range allWorklogs {
		record := worklog.ToRecord(fields)
		if err := writer.Write(record); err != nil {
			log.Fatalf("writer.Write error: %v\nrecord=[%v]\n", err, record)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("writer.Error error: %v\n", err)
	}
}

func getWorklogResult(baseURL url.URL, key string, queryParams url.Values) (*WorklogResult, error) {

	worklogURL := baseURL
	worklogURL.Path = fmt.Sprintf("/rest/api/%s/issue/%s/worklog", ApiVersion, key)
	worklogURL.RawQuery = queryParams.Encode()
	req, err := http.NewRequest("GET", worklogURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %v\nworklogURL=[%v]", err, worklogURL)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.URLEncoding.EncodeToString([]byte(AuthUser+":"+AuthToken))))
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

	baseURL, err := url.Parse(BaseURL)
	if err != nil {
		return nil, fmt.Errorf("url.Parse error: %v\njiraURL=[%v]", err, BaseURL)
	}

	queryParams := url.Values{
		"startAt":    []string{"0"},
		"maxResults": []string{"1048576"},
		"startedAfter": []string{func() string {
			t := time.Now().AddDate(0, -1, 0)
			if len(TargetYearMonth) > 0 {
				if v, err := time.Parse("2006-01-02", TargetYearMonth+"-01"); err == nil {
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

	if result.IsNotEmpty() {
		return result, nil
	}

	return nil, fmt.Errorf("empty result")
}

func startOfMonthEpocMillis(t time.Time) string {
	startOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	return fmt.Sprintf("%d", startOfMonth.UnixNano()/int64(time.Millisecond))
}
