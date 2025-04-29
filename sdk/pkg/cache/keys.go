package cache

import "fmt"

var (
	AuthDetails        string = "authDetails"
	PriorityData       string = "priorityData"
	SubLendersData     string = "subLendersData"
	RcsTemplateAppData string = "rcsTemplateAppData"
)

func GetRankKey(subLenderId int) string {
	return fmt.Sprintf("%v_priority", subLenderId)
}
