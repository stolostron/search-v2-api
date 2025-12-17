package resolver

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"k8s.io/utils/strings/slices"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/lib/pq"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/rbac"
	"k8s.io/klog/v2"
)

func getPropertyType(ctx context.Context, refresh bool) (map[string]string, error) {
	propTypesCache, err := rbac.GetCache().GetPropertyTypes(ctx, refresh)
	return propTypesCache, err
}

// Extract operator (<=, >=, !=, !, <, >, =) if any from string
func getOperatorFromString(value string) (string, string) {
	operator := "="
	operand := value

	prefixes := []string{"<=", ">=", "!=", "!", "<", ">", "="}
	for _, prefix := range prefixes {
		if cutString, yes := strings.CutPrefix(value, prefix); yes {
			operator = prefix
			operand = cutString
			klog.V(5).Infof("Extracted operator: %s and operand: %s from value: %s", operator, operand, value)
			break
		}
	}
	return operator, operand
}

// Extract operator (<=, >=, !=, !, <, >, =) if any from values. Combine with other operators like "*" if present.
func extractOperator(values []string, innerOperator string,
	operatorOperandMap map[string][]string) map[string][]string {
	for _, value := range values {
		operator, operand := getOperatorFromString(value)
		if innerOperator != "" {
			updateOperatorValueMap(operator+":"+innerOperator, operatorOperandMap, operand)
		} else {
			updateOperatorValueMap(operator, operatorOperandMap, operand)
		}
	}
	return operatorOperandMap
}

// Check if value has partial match "*"
// Returns a map that stores operator and values
func getPartialMatchFilter(filter string, values []string, dataType interface{},
	operatorOperandMap map[string][]string) map[string][]string {
	for i, val := range values {
		if strings.Contains(val, "*") {
			values[i] = strings.ReplaceAll(val, "*", "%")
		}
	}
	switch dataType {
	case "object":
		return extractOperator(values, "*@>", operatorOperandMap)
	case "array":
		return extractOperator(values, "*[]", operatorOperandMap)
	default:
		return extractOperator(values, "*", operatorOperandMap)
	}
}

// compareValues checks if a string is equal to any string in an array of strings.
func compareValues(inputArray, compareArray []string) bool {
	for _, date := range compareArray {
		for _, str := range inputArray {
			if strings.Contains(str, date) {
				return true
			}
		}
	}
	return false
}

func getWhereClauseExpression(prop, operator string, values []string, dataType string) []exp.Expression {
	klog.V(5).Info("Building where clause for filter: ", prop, " with ", len(values), " values: ", values,
		"and operator: ", operator)
	exps := []exp.Expression{}
	var lhsExp interface{}

	// check if the property is cluster
	switch prop {
	case "cluster":
		lhsExp = goqu.C(prop)
	case "managedHub":
		// managedHub is not a property in the database. This filter is used to federate the request to this hub
		// This property is used to federate the request to this specific hub.
		// So, fetch results based on the other filters.
		return exps
	default:
		lhsExp = goqu.L(`"data"->>?`, prop)
		if dataType == "number" {
			lhsExp = goqu.L(`("data"->?)?`, prop, goqu.L("::numeric"))
		}
	}
	switch operator {
	case "*", "=:*":
		for _, val := range values {
			exps = append(exps, goqu.L(`?`, lhsExp).Like(val))
		}
	case "!:*", "!=:*":
		for _, val := range values {
			exps = append(exps, goqu.L("NOT(?)", goqu.L(`?`, lhsExp).Like(val)))
		}
	case "<=":
		for _, val := range values {
			exps = append(exps, goqu.L(`?`, lhsExp).Lte(val))
		}
	case ">=":
		for _, val := range values {
			exps = append(exps, goqu.L(`?`, lhsExp).Gte(val))
		}
	case "!=":
		exps = append(exps, goqu.L(`?`, lhsExp).Neq(values))

	case "!":
		exps = append(exps, goqu.L(`?`, lhsExp).NotIn(values))

	case "<":
		for _, val := range values {
			exps = append(exps, goqu.L(`?`, lhsExp).Lt(val))
		}
	case ">":
		for _, val := range values {
			exps = append(exps, goqu.L(`?`, lhsExp).Gt(val))
		}
	case "!:*@>", "!=:*@>":
		exps = append(exps, goqu.L("NOT EXISTS(?)", createSubQueryForArray("object", prop, values)))

	case ":*@>", "=:*@>":
		exps = append(exps, goqu.L("EXISTS(?)", createSubQueryForArray("object", prop, values)))

	case "!:*[]", "!=:*[]":
		exps = append(exps, goqu.L("NOT EXISTS(?)", createSubQueryForArray("array", prop, values)))

	case ":*[]", "=:*[]":
		exps = append(exps, goqu.L("EXISTS(?)", createSubQueryForArray("array", prop, values)))

	case "@>", "=:@>":
		for _, val := range values {
			exps = append(exps, goqu.L(`"data"->? @> ?`, prop, val))
		}
	case "!:@>", "!=:@>":
		for _, val := range values {
			exps = append(exps, goqu.L("NOT(?)", goqu.L(`"data"->? @> ?`, prop, val)))
		}
	case "?|":
		exps = append(exps, goqu.L(`"data"->? ? ?`, prop, "?|", values))
	default:
		if prop == "kind" && isLower(values) {
			//ILIKE to enable case-insensitive comparison for kind. Needed for V1 compatibility.
			exps = append(exps, goqu.L(`"data"->>?`, prop).ILike(goqu.Any(pq.Array(values))))
			klog.Warning("Using ILIKE for lower case KIND string comparison.",
				"- This behavior is needed for V1 compatibility and will be deprecated with Search V2.")
		} else if isString(values) && prop != "cluster" {
			if len(values) == 1 { // for single value, use "?" operator
				// Refer to https://www.postgresql.org/docs/9.5/functions-json.html#FUNCTIONS-JSONB-OP-TABLE
				lhsExp = goqu.L(`"data"->?`, prop)
				exps = append(exps, goqu.L("???", lhsExp, goqu.Literal("?"), values))
			} else { // if there are many values, use "?|" operator
				lhsExp = goqu.L(`"data"->?`, prop)
				exps = append(exps, goqu.L("???", lhsExp, goqu.Literal("?|"), pq.Array(values)))
			}
		} else {
			exps = append(exps, goqu.L(`?`, lhsExp).In(values))
		}
	}
	return exps

}

func createSubQueryForArray(dataType, prop string, values []string) *goqu.SelectDataset {
	var subexps []exp.Expression

	if dataType == "array" {
		for _, val := range values {
			subexps = append(subexps, goqu.L(`arrayProp`).Like(val))
		}
		return goqu.From(goqu.L(`jsonb_array_elements_text("data"->?) As arrayProp`, prop)).
			Select(goqu.L("1")).
			Where(goqu.Or(subexps...))
	} else {
		var subexpInnerList []exp.Expression
		var subexpList exp.ExpressionList
		for _, val := range values {
			keyValue := strings.Split(val, ":")
			if len(keyValue) == 2 {
				subexps := []exp.Expression{goqu.L(`key`).Like(keyValue[0]), goqu.L(`value`).Like(keyValue[1])}
				subexpInnerList = append(subexpInnerList, goqu.And(subexps...))
			} else {
				subexps := []exp.Expression{goqu.L(`key`).Like(keyValue), goqu.L(`value`).Like(keyValue)}
				subexpInnerList = append(subexpInnerList, goqu.Or(subexps...))
			}
			subexpList = goqu.Or(subexpInnerList...)
		}
		return goqu.From(goqu.L(`jsonb_each_text("data"->?) As kv(key, value)`, prop)).
			Select(goqu.L("1")).
			Where(goqu.Or(subexpList))
	}
}

// Check if the values contain numerical values
func isString(values []string) bool {
	for _, v := range values {
		if _, err := strconv.ParseInt(v, 10, 64); err == nil {
			klog.V(6).Infof("value array contains a number %q .\n", v)
			return false
		}
	}
	return true
}

// if any string values starts with lower case letters, return true
func isLower(values []string) bool {
	for _, str := range values {
		firstChar := rune(str[0]) //check if first character of the string is lower case
		if unicode.IsLower(firstChar) && unicode.IsLetter(firstChar) {
			return true
		}
	}
	return false
}

// Store the operator and value in a map - this is to handle multiple values
func updateOperatorValueMap(operator string, operatorValueMap map[string][]string,
	operatorRemovedValue string) map[string][]string {
	if vals, ok := operatorValueMap[operator]; !ok {
		operatorValueMap[operator] = []string{operatorRemovedValue} // Add an entry to map with key as operator
	} else {
		vals = append(vals, operatorRemovedValue)
		operatorValueMap[operator] = vals
	}
	return operatorValueMap
}

// Check if value is a date and get the operator
// Returns a map that stores operator and values
func getOperatorIfDateFilter(filter string, values []string,
	opValueMap map[string][]string) map[string][]string {
	now := time.Now()
	for _, val := range values {
		operator, operand := getOperatorFromString(val)
		if operator == "=" { // For dates, unless specified otherwise, always check for values '>'
			operator = ">"
		}
		var then string
		format := "2006-01-02T15:04:05Z"
		switch operand {
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
		}
		// Add the value and operator to map
		updateOperatorValueMap(operator, opValueMap, then)
	}
	return opValueMap
}

// formatMap converts a map to a string sorted by keys alphabetically in the following format:
// key1:value1; key2:value2; ..."
func formatMap(labels map[string]interface{}) string {
	keys := make([]string, 0, len(labels))
	valueStrings := make([]string, 0, len(labels))

	for k := range labels {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		var val string

		if valAsFloat, ok := labels[k].(float64); ok {
			// Converting the float64 to an int64 follows the convention in formatDataMap.
			val = strconv.FormatInt(int64(valAsFloat), 10)
		} else {
			val = fmt.Sprintf("%s", labels[k])
		}

		valueStrings = append(valueStrings, fmt.Sprintf("%s=%s", k, val))
	}

	return strings.Join(valueStrings, "; ")
}

// Encode array into a single string with the format.
//
//	value1; value2; ...
func formatArray(itemlist []interface{}) string {
	keys := make([]string, len(itemlist))
	for i, k := range itemlist {
		keys[i] = convertToString(k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "; ")
}

// Convert interface to string format
func convertToString(data interface{}) string {
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

func formatDataMap(data map[string]interface{}) map[string]interface{} {
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
			item[key] = formatMap(v)
		case []interface{}:
			item[key] = formatArray(v)
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

func CheckIfInArray(lookupMap []string, uid string) bool {
	for _, id := range lookupMap {
		if id == uid {
			return true
		}
	}
	return false
}

func decodeObject(isPartialMatch bool, values []string) ([]string, error) {
	cleanedVal := make([]string, len(values))

	for i, val := range values {
		operator, operand := getOperatorFromString(val)
		labels := strings.Split(operand, "=")
		if len(labels) == 2 {
			if isPartialMatch {
				cleanedVal[i] = fmt.Sprintf(`%s%s:%s`, operator, labels[0], labels[1])
			} else {
				cleanedVal[i] = fmt.Sprintf(`%s{"%s":"%s"}`, operator, labels[0], labels[1])
			}
		} else {
			if isPartialMatch {
				cleanedVal[i] = fmt.Sprintf(`%s%s`, operator, labels[0])
			} else {
				return cleanedVal,
					fmt.Errorf("incorrect label format, label filters must have the format key=value")
			}
		}

	}
	return cleanedVal, nil
}

func decodeArray(isPartialMatch bool, values []string) ([]string, error) {
	cleanedVal := make([]string, len(values))

	for i, val := range values {
		operator, operand := getOperatorFromString(val)

		if !isPartialMatch {
			cleanedVal[i] = fmt.Sprintf(`%s["%s"]`, operator, operand)
		} else {
			cleanedVal[i] = val
		}
	}
	return cleanedVal, nil
}

func decodePropertyTypes(values []string, dataType string) ([]string, error) {
	isPartialMatch := compareValues(values, []string{"*"})

	switch dataType {
	case "object":
		return decodeObject(isPartialMatch, values)
	case "array":
		return decodeArray(isPartialMatch, values)
	default:
		return values, nil
	}
}

func getKeys(stringKeyMap interface{}) []string {
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
func (s *SearchResult) setLimit() uint {
	var limit uint
	if s.input != nil && s.input.Limit != nil && *s.input.Limit > 0 {
		if *s.input.Limit <= math.MaxUint32 {
			limit = uint(*s.input.Limit) // #nosec G115
		} else {
			limit = math.MaxUint32
		}
	} else if s.input != nil && s.input.Limit != nil && *s.input.Limit == -1 {
		klog.V(2).Info("No limit set on query. Fetching all results.")
	} else {
		limit = config.Cfg.QueryLimit
	}
	return limit
}

func matchOperatorToProperty(dataType string, opValueMap map[string][]string,
	values []string, property string) map[string][]string {
	if (dataType == "object" || dataType == "array") && !compareValues(values, []string{"*"}) {
		opValueMap = extractOperator(values, "@>", opValueMap)
	} else if compareValues(values, []string{"hour", "day", "week", "month", "year"}) {
		// Check if value is a number or date and get the cleaned up value
		opValueMap = getOperatorIfDateFilter(property, values, opValueMap)
	} else if compareValues(values, []string{"*"}) { //partialMatch
		opValueMap = getPartialMatchFilter(property, values, dataType, opValueMap)
	} else {
		opValueMap = extractOperator(values, "", opValueMap)
	}
	return opValueMap
}

// partialMatchStringPattern checks if config.Cfg.HubName partially matches any pattern in the values slice.
// It loops through each pattern, prepares it for matching, and checks for a match.
// If a match is found, it returns true, indicating a match is found. Else, returns false.
func partialMatchStringPattern(values []string) (bool, error) {
	for _, pattern := range values {
		klog.V(5).Info("ManagedHub filter pattern to match: ", pattern, " hubname: ", config.Cfg.HubName)
		//fix prefix
		if !strings.HasPrefix(pattern, "*") {
			pattern = "^" + pattern
		}
		//fix suffix
		if !strings.HasSuffix(pattern, "*") {
			pattern = pattern + "$"
		}
		// fix start and end of string and match multiple characters
		pattern = strings.ReplaceAll(pattern, "%", ".*")
		matched, err := regexp.MatchString(pattern, config.Cfg.HubName)

		if err != nil {
			klog.Error("Error in partialMatchStringPattern for managedHub filter:", err)
			return false, err
		}
		if matched {
			return matched, nil
		}
	}
	klog.V(4).Infof("%s not in managedHub filter %+v", config.Cfg.HubName, values)
	return false, nil
}

// processOpValueMapManagedHub processes the key-value pair for a managedHub filter.
// It handles different key cases such as "!", "!=", "=", "!:*", "!=:*", and "=:*".
// It returns a boolean indicating whether the search should proceed based on the evaluation of the key and values.
func processOpValueMapManagedHub(key string, values []string) bool {
	result := false
	switch key {
	// Check if config.Cfg.HubName is in the values slice
	case "!", "!=":
		result = !slices.Contains((values), config.Cfg.HubName) // Search should not proceed if there is a match
	case "=":
		result = slices.Contains(values, config.Cfg.HubName) // Search to proceed if there is a match
	case "!:*", "!=:*":
		match, err := partialMatchStringPattern(values)
		if err != nil {
			klog.Error("Error processing partial match for ManagedHub filter:", err)
			return false
		}
		result = !match // Return the inverse of match to indicate search should not proceed if there is a partial match
	case "=:*":
		match, err := partialMatchStringPattern(values)
		if err != nil {
			klog.Error("Error processing partial match for ManagedHub filter:", err)
			return false
		}
		result = match // Return match to indicate search should proceed if there is a partial match
	}
	klog.V(4).Infof("ManagedHub filter hubname: %s operation: %s values: %+v  result: %t",
		config.Cfg.HubName, key, values, result)

	return result
}
