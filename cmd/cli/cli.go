package cli

import (
	"bitbucket.org/yujiorama/jira-timespent-report/jira"
	"log"
	"os"
)

func Do() {
	log.Println("start")
	jira.SetFlags()
	issues, worklogs, searchErrors := jira.Search()
	for _, err := range searchErrors {
		log.Printf("%v\n", err)
	}

	reportErrors := jira.Report(os.Stdout, issues, worklogs)
	if reportErrors != nil {
		for _, err := range reportErrors {
			log.Printf("%v\n", err)
		}
	}

	log.Println("end")
}
