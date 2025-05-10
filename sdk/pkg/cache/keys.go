package cache

import "fmt"

var (
	AuthDetails         string = "authDetails"
	PriorityData        string = "priorityData"
	VendorsData         string = "vendorsData"
	ClientsData         string = "clientsData"
	TemplateDetailsData string = "templateDetailsData"
	ActiveVendors       string = "activeVendors"
	RcsTemplateAppData  string = "rcsTemplateAppData"
)

func GetRankKey(subLenderId int) string {
	return fmt.Sprintf("%v_priority", subLenderId)
}

func GetVendorKey(vendorName, channelName string) string {
	return fmt.Sprintf("%s_%s", vendorName, channelName)
}
