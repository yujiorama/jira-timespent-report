package cli

import (
	"log"
	"os"

	"bitbucket.org/yujiorama/jira-timespent-report/jira"
)

func Do() {
	log.Println("start")

	issues, worklogs, searchErrors := jira.Search()
	for _, err := range searchErrors {
		log.Printf("%v\n", err)
	}

	reportErrors := jira.Report(os.Stdout, issues, worklogs)
	for _, err := range reportErrors {
		log.Printf("%v\n", err)
	}

	log.Println("end")
}
