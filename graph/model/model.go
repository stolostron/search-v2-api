package model

import (
	"fmt"
	"strings"
)

func (input *SearchInput) String() string {
	str := "SearchInput {\n"
	// Keywords
	str += fmt.Sprintf("\tkeywords: [%s]\n", strings.Join(stringPtrArrayToStringArray(input.Keywords), ","))

	// Filters
	filterStrings := make([]string, len(input.Filters))
	for i, filter := range input.Filters {
		filterStrings[i] = fmt.Sprintf("{ property: %s, \tvalues: [%s] }", filter.Property, strings.Join(stringPtrArrayToStringArray(filter.Values), ", "))
	}
	str += fmt.Sprintf("\tfilters: [\n\t\t%s\n\t],\n", strings.Join(filterStrings, ",\n\t\t"))

	// Related Kinds
	str += fmt.Sprintf("\trelatedKinds: [%s],\n", strings.Join(stringPtrArrayToStringArray(input.RelatedKinds), ", "))

	// Limit
	if input.Limit != nil {
		str += fmt.Sprintf("\tlimit: %d", *input.Limit)
	} else {
		str += "\tlimit: nil"
	}
	str += "\n}"
	return str
}

func stringPtrArrayToStringArray(s []*string) []string {
	stringArray := make([]string, len(s))
	for i, ptr := range s {
		stringArray[i] = *ptr
	}
	return stringArray
}
