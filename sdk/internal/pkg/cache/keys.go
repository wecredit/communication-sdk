package cache

import "fmt"

var (
	AuthDetails        string = "authDetails"
	PriorityData       string = "priorityData"
	SubLendersData     string = "subLendersData"
	LendersData        string = "lendersData"
	AbflPincodes       string = "abflPinodes"
	PoonawallaPincodes string = "poonawallaPinodes"
	UnityPincodes      string = "unityPinodes"
	PrefrPincodes      string = "prefrPinodes"
	AbflBlPincodes     string = "abflBlPinodes"
)

func GetRankKey(subLenderId int) string {
	return fmt.Sprintf("%v_priority", subLenderId)
}