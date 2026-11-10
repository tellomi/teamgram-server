package location

import "strings"

// VenueType maps an AMap typecode to a Foursquare category path. Official
// clients build the row icon from that path on ss3.4sqi.net. The host is
// fixed in the client, so the path has to be one that CDN already serves.
func VenueType(typecode string) string {
	typecode = strings.TrimSpace(typecode)
	switch {
	case strings.HasPrefix(typecode, "1505"):
		return "travel/airport"
	case strings.HasPrefix(typecode, "1502"):
		return "travel/trainstation"
	case strings.HasPrefix(typecode, "1503"):
		return "travel/subway"
	case strings.HasPrefix(typecode, "0505"):
		return "food/cafe"
	}
	prefix := typecode
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	switch prefix {
	case "05":
		return "food/default"
	case "06", "07":
		return "shops/default"
	case "08":
		return "arts_entertainment/default"
	case "09":
		return "building/medical"
	case "10":
		return "travel/hotel"
	case "11":
		return "parks_outdoors/default"
	case "12":
		return "building/default"
	case "13":
		return "building/government_capitolbuilding"
	case "14":
		return "education/default"
	case "15":
		return "travel/default"
	case "16":
		return "shops/financial"
	default:
		return "building/default"
	}
}

// TencentVenueType maps a Tencent category ("美食:中餐厅") onto the same paths.
func TencentVenueType(category string) string {
	head, _, _ := strings.Cut(category, ":")
	head = strings.TrimSpace(head)
	switch head {
	case "美食":
		return "food/default"
	case "购物", "生活服务":
		return "shops/default"
	case "酒店":
		return "travel/hotel"
	case "基础设施", "交通设施":
		return "travel/default"
	case "教育学校", "教育":
		return "education/default"
	case "医疗保健", "医疗":
		return "building/medical"
	case "旅游景点", "风景名胜":
		return "parks_outdoors/default"
	case "运动健身", "体育休闲":
		return "arts_entertainment/default"
	case "房产小区", "商务住宅":
		return "building/default"
	case "机构团体", "政府机构":
		return "building/government_capitolbuilding"
	case "银行金融", "金融":
		return "shops/financial"
	default:
		if strings.Contains(head, "机场") {
			return "travel/airport"
		}
		if strings.Contains(head, "地铁") {
			return "travel/subway"
		}
		if strings.Contains(head, "火车") || strings.Contains(head, "火车站") {
			return "travel/trainstation"
		}
		return "building/default"
	}
}

// AppleVenueType maps an Apple Maps POI category onto a Foursquare path.
// Official clients load the row icon from ss3.4sqi.net using that path
// (categories_v2/<path>_64.png on Android, _88.png on iOS) and pick the
// row color from the first segment.
func AppleVenueType(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "cafe":
		return "food/cafe"
	case "restaurant", "bakery", "brewery", "winery", "foodmarket":
		return "food/default"
	case "hotel":
		return "travel/hotel"
	case "park", "nationalpark", "beach", "campground":
		return "parks_outdoors/default"
	case "store":
		return "shops/default"
	case "nightlife":
		return "nightlife/default"
	case "hospital", "pharmacy":
		return "building/medical"
	case "school", "university", "library":
		return "education/default"
	case "museum", "movietheater", "theater", "amusementpark", "aquarium", "zoo":
		return "arts_entertainment/default"
	case "airport":
		return "travel/airport"
	case "publictransport", "carrental", "gasstation", "marina":
		return "travel/default"
	case "fitnesscenter", "stadium":
		return "building/gym"
	case "bank", "atm":
		return "shops/financial"
	default:
		return "building/default"
	}
}
