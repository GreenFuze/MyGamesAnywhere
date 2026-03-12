package db

// testDBConfig implements core.Configuration for tests. Uses a temp file for DB_PATH when not set.
type testDBConfig struct {
	dbPath string
}

func (c testDBConfig) Get(key string) string {
	if key == "DB_PATH" {
		if c.dbPath != "" {
			return c.dbPath
		}
		return "file::memory:?cache=shared"
	}
	return ""
}

func (c testDBConfig) GetInt(key string) int   { return 0 }
func (c testDBConfig) GetBool(key string) bool { return false }
func (c testDBConfig) Validate() error         { return nil }

// testLogger implements core.Logger for tests (no-op).
type testLogger struct{}

func (testLogger) Info(msg string, args ...any)   {}
func (testLogger) Error(msg string, err error, args ...any) {}
func (testLogger) Debug(msg string, args ...any)  {}
func (testLogger) Warn(msg string, args ...any)   {}
