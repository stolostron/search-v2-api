package resolver

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"k8s.io/klog/v2"
)

func GetPropertyTypeCache(ctx context.Context) (map[string]string, error) {
	propTypesCache, err := rbac.GetCache().GetPropertyTypes(ctx)
	return propTypesCache, err
}

// Remove operator (<=, >=, !=, !, <, >, =) if any from values
func GetOperator(values []string) map[string][]string {
	// Get the operator (/^<=|^>=|^!=|^!|^<|^>|^=/)
	var operator string
	// Replace any of these symbols with ""
	replacer := strings.NewReplacer("<=", "",
		">=", "",
		"!=", "",
		"!", "",
		"<", "",
		">", "",
		"=", "")
	operatorValue := map[string][]string{}

	for _, value := range values {
		operatorRemovedValue := replacer.Replace(value)
		operator = strings.Replace(value, operatorRemovedValue, "", 1) // find operator
		if vals, ok := operatorValue[operator]; !ok {
			if operator != "" { // Add to map only if operator is present
				operatorValue[operator] = []string{operatorRemovedValue} // Add an entry to map with key as operator
			}
		} else {
			vals = append(vals, operatorRemovedValue)
			operatorValue[operator] = vals
		}
	}
	return operatorValue
}

func GetWhereClauseExpression(prop, operator string, values []string, propType string) []exp.Expression {
	exps := []exp.Expression{}

	switch operator {
	case "<=":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Lte(val))
		}
	case ">=":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Gte(val))
		}
	case "!=":
		exps = append(exps, goqu.L(`"data"->>?`, prop).Neq(values))

	case "!":
		exps = append(exps, goqu.L(`"data"->>?`, prop).NotIn(values))
	case "<":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Lt(val))
		}
	case ">":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->>?`, prop).Gt(val))
		}
	case "=":
		exps = append(exps, goqu.L(`"data"->>?`, prop).In(values))

	case "@>":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->? @> ?`, prop, val))
		}
	case "?|":
		exps = append(exps, goqu.L(`"data"->? ? ?`, prop, "?|", values))

	default:
		if prop == "cluster" {
			exps = append(exps, goqu.C(prop).In(values))
		} else if prop == "kind" { //ILIKE to enable case-insensitive comparison for kind. Needed for V1 compatibility.
			if IsLower(values) {
				exps = append(exps, goqu.L(`"data"->>?`, prop).ILike(goqu.Any(pq.Array(values))))
				klog.Warning("Using ILIKE for lower case KIND string comparison.",
					"- This behavior is needed for V1 compatibility and will be deprecated with Search V2.")
			} else {
				exps = append(exps, goqu.L(`"data"->>?`, prop).In(values))
			}
		} else {
			exps = append(exps, goqu.L(`"data"->>?`, prop).In(values))
		}
	}
	return exps

}

//if any string values starts with lower case letters, return true
func IsLower(values []string) bool {
	for _, str := range values {
		firstChar := rune(str[0]) //check if first character of the string is lower case
		if unicode.IsLower(firstChar) && unicode.IsLetter(firstChar) {
			return true
		}
	}
	return false
}

// Check if value is a number or date and get the operator
// Returns a map that stores operator and values
func GetOperatorAndNumDateFilter(filter string, values []string, dataType interface{}) map[string][]string {
	opValueMap := GetOperator(values) //If values are numbers

	// Store the operator and value in a map - this is to handle multiple values
	updateOpValueMap := func(operator string, operatorValueMap map[string][]string, operatorRemovedValue string) {
		if vals, ok := operatorValueMap[operator]; !ok {
			operatorValueMap[operator] = []string{operatorRemovedValue}
		} else {
			vals = append(vals, operatorRemovedValue)
			operatorValueMap[operator] = vals
		}
	}

	if len(opValueMap) < 1 { //If not a number (no operator), check if values are dates
		// Expected values: {"hour", "day", "week", "month", "year"}
		operator := ">" // For dates, always check for values '>'
		now := time.Now()
		for _, val := range values {

			var then string
			format := "2006-01-02T15:04:05Z"
			switch val {
			case "hour":
				then = now.Add(time.Duration(-1) * time.Hour).Format(format)

			case "day":
				then = now.AddDate(0, 0, -1).Format(format)

			case "week":
				then = now.AddDate(0, 0, -7).Format(format)

			case "month":
				then = now.AddDate(0, -1, 0).Format(format)

			case "year":
				then = now.AddDate(-1, 0, 0).Format(format)

			default:
				//check that property value is an array:
				if dataType == "object" || dataType == "array" {
					klog.V(7).Info("filter is object or array type. Operator is @>.")
					operator = "@>"

				} else {
					klog.V(7).Info("filter is neither label nor in arrayProperties: ", filter)
					operator = ""
				}

				then = val
			}
			// Add the value and operator to map
			updateOpValueMap(operator, opValueMap, then)
		}
	}
	return opValueMap
}

// Labels are sorted alphabetically to ensure consistency, then encoded in a
// string with the following format.
// key1:value1; key2:value2; ...
func FormatLabels(labels map[string]interface{}) string {
	keys := make([]string, 0)
	labelStrings := make([]string, 0)
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		labelStrings = append(labelStrings, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(labelStrings, "; ")
}

// Encode array into a single string with the format.
//  value1; value2; ...
func FormatArray(itemlist []interface{}) string {
	keys := make([]string, len(itemlist))
	for i, k := range itemlist {
		keys[i] = ConvertToString(k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "; ")
}

// Convert interface to string format
func ConvertToString(data interface{}) string {
	var item string
	switch v := data.(type) {
	case string:
		item = strings.ToLower(v)
	case bool:
		item = strconv.FormatBool(v)
	case float64:
		item = strconv.FormatInt(int64(v), 10)
	default:
		klog.Warningf("Error formatting property with type: %+v\n", reflect.TypeOf(v))
	}
	return item
}

func FormatDataMap(data map[string]interface{}) map[string]interface{} {
	item := make(map[string]interface{})
	for key, value := range data {
		switch v := value.(type) {
		case string:
			item[key] = v //strings.ToLower(v)
		case bool:
			item[key] = strconv.FormatBool(v)
		case float64:
			item[key] = strconv.FormatInt(int64(v), 10)
		case map[string]interface{}:
			item[key] = FormatLabels(v)
		case []interface{}:
			item[key] = FormatArray(v)
		default:
			klog.Warningf("Error formatting property with key: %+v  type: %+v\n", key, reflect.TypeOf(v))
			continue
		}
	}
	return item
}

// helper function to point values in string  array
func PointerToStringArray(pointerArray []*string) []string {

	values := make([]string, len(pointerArray))
	for i, val := range pointerArray {

		values[i] = *val

	}
	return values
}

func DecodePropertyTypes(values []string, dataTypeFromMap string) ([]string, string) {
	dataType := dataTypeFromMap
	cleanedVal := make([]string, len(values))

	for i, val := range values {
		if dataType == "object" {
			labels := strings.Split(val, "=")
			cleanedVal[i] = fmt.Sprintf(`{"%s":"%s"}`, labels[0], labels[1])
		} else if dataType == "array" {
			cleanedVal[i] = fmt.Sprintf(`["%s"]`, val)

		} else {
			cleanedVal[i] = val
		}

		values = cleanedVal
	}
	return values, dataType

}

func GetKeys(stringKeyMap interface{}) []string {
	v := reflect.ValueOf(stringKeyMap)
	if v.Kind() != reflect.Map {
		klog.Error("input in getKeys is not a map")
	}
	if v.Type().Key().Kind() != reflect.String {
		klog.Error("input map in getKeys does not have string keys")
	}
	keys := make([]string, 0, v.Len())
	for _, key := range v.MapKeys() {
		keys = append(keys, key.String())
	}
	sort.Strings(keys)
	return keys
}

// Set limit for queries
func (s *SearchResult) SetLimit() int {
	var limit int
	if s.input != nil && s.input.Limit != nil && *s.input.Limit > 0 {
		limit = *s.input.Limit
	} else if s.input != nil && s.input.Limit != nil && *s.input.Limit == -1 {
		klog.Warning("No limit set. Fetching all results.")
	} else {
		limit = config.Cfg.QueryLimit
	}
	return limit
}
