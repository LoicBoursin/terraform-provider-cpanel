package utils

import "github.com/hashicorp/terraform-plugin-framework/types"

func SliceUniqueTypesString(s []types.String) []types.String {
	unique := make(map[types.String]bool)
	var us []types.String
	for _, elem := range s {
		if _, ok := unique[elem]; !ok {
			us = append(us, elem)
			unique[elem] = true
		}
	}
	return us
}
