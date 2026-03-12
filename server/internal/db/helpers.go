package db

func buildPlaceholders(n int) string {
	if n == 0 {
		return ""
	}
	s := "?"
	for i := 1; i < n; i++ {
		s += ",?"
	}
	return s
}

func nullEmptyStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullEmpty is an alias for nullEmptyStr for use in repos that need a string->interface{} for nullable columns.
func nullEmpty(s string) interface{} {
	return nullEmptyStr(s)
}
