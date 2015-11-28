// This packages sole purpose is to parse some config file (defaults to Go program calling it basename.)
// Ideally should be abstracted out more to parse any YAML, TOML based config file and return the struct.
package cfg

import (
    "fmt"
    "reflect"
    "strings"
    "time"

    "github.com/kr/pretty"
    "github.com/mitchellh/mapstructure"
    "github.com/spf13/cast"
    jww "github.com/spf13/jwalterweatherman"
)

var c *Config

func init() {
    c = New()
}

type Config struct {
    // Delimiter used to access sub keys in a single command
    keyDelm string

    // Name of file to look for in paths
    configName string
    configFile string
    configType string

    // List of to search for files
    configPaths []string

    config   map[string]interface{}
    defaults map[string]interface{}
    aliases  map[string]string

    typeByDefValue bool
}

// Denotes finding an unsurpported config
type UnsupportedConfigError string

// Returns the error for an unsurpported config
func (str UnsupportedConfigError) Error() string {
    return fmt.Sprintf("Unsurpported Config Type %q", string(str))
}

// Denotes failing to find configuration file.
type ConfigFileNotFoundError struct {
    name, locations string
}

// Returns the formatted configuration error.
func (fnfe ConfigFileNotFoundError) Error() string {
    return fmt.Sprintf("Config File %q Not Found in %q", fnfe.name, fnfe.locations)
}

// Universally supported extensions.
var SupportedExts []string = []string{"json", "toml", "yaml", "yml", "properties", "props", "prop"}

// Returns a properly initialized Config instance
func New() *Config {
    c := new(Config)
    c.keyDelm = "."
    c.configName = "config"
    c.config = make(map[string]interface{})
    c.defaults = make(map[string]interface{})
    c.aliases = make(map[string]string)
    c.typeByDefValue = false

    return c
}

// Explicitly sets the config file to be used.
func SetConfigFile(s string) { c.SetConfigFile(s) }
func (c *Config) SetConfigFile(s string) {
    if s != "" {
        c.configFile = s
    }
}

// Return the file used to populate the config.
func ConfigFileUsed() string             { return c.ConfigFileUsed() }
func (c *Config) ConfigFileUsed() string { return c.configFile }

// Adds a path to search for the config files to load.
//
// Function takes a string parameter and adds the path to search to an array. This ordered array
// Determines the search path for configuration files in order of presedence.
// This function does NOT check whether the path is valid at the time it is being added.
func AddConfigPath(s string) { c.AddConfigPath(s) }
func (c *Config) AddConfigPath(s string) {
    if s != "" {
        inPath := absPathify(s)
        jww.INFO.Println("adding ", inPath, " to search paths.")
        if !stringInSlice(inPath, c.configPaths) {
            c.configPaths = append(c.configPaths, inPath)
        }
    }
}

func (c *Config) searchMap(s map[string]interface{}, p []string) interface{} {
    if len(p) == 0 {
        return s
    }

    if next, ok := s[p[0]]; ok {
        switch next.(type) {
        case map[interface{}]interface{}:
            return c.searchMap(cast.ToStringMap(next), p[1:])
        case map[string]interface{}:
            return c.searchMap(next.(map[string]interface{}), p[1:])
        default:
            return next
        }
    } else {
        return nil
    }
}

func Get(key string) interface{} { c.Get(key) }
func (c *Config) Get(key string) interface{} {
    p := strings.Split(key, c.keyDelm)

    lcaseKey := strings.ToLower(key)
    val := c.find(lcaseKey)

    if val == nil {
        source := c.find(p[0])
        if source != nil {
            if reflect.TypeOf(source).Kind() == reflect.Map {
                val = c.searchMap(cast.ToStringMap(source), p[1:])
            }
        }
    }

    if val == nil {
        return nil
    }

    var valType interface{}
    if !c.typeByDefValue {
        valType = val
    } else {
        defVal, defExists := c.defaults[lcaseKey]
        if defExists {
            valType = defVal
        } else {
            valType = val
        }
    }

    switch valType.(type) {
    case bool:
        return cast.ToBool(val)
    case string:
        return cast.ToString(val)
    case int64, int32, int16, int8, int:
        return cast.ToInt(val)
    case float64, float32:
        return cast.ToFloat64(val)
    case time.Time:
        return cast.ToTime(val)
    case time.Duration:
        return cast.ToDuration(val)
    case []string:
        return cast.ToStringSlice(val)
    }

    return val
}

// Returns the value associated with the key as a string
func GetString(key string) string { return c.GetString(key) }
func (c *Config) GetString(key string) string {
    return cast.ToString(c.Get(key))
}

// Returns the value associated with the key asa boolean
func GetBool(key string) bool { return c.GetBool(key) }
func (c *Config) GetBool(key string) bool {
    return cast.ToBool(c.Get(key))
}

// Returns the value associated with the key as an integer
func GetInt(key string) int { return c.GetInt(key) }
func (c *Config) GetInt(key string) int {
    return cast.ToInt(c.Get(key))
}

// Returns the value associated with the key as a float64
func GetFloat64(key string) float64 { return c.GetFloat64(key) }
func (c *Config) GetFloat64(key string) float64 {
    return cast.ToFloat64(c.Get(key))
}

// Returns the value associated with the key as time
func GetTime(key string) time.Time { return c.GetTime(key) }
func (c *Config) GetTime(key string) time.Time {
    return cast.ToTime(c.Get(key))
}

// Returns the value associated with the key as a duration
func GetDuration(key string) time.Duration { return c.GetDuration(key) }
func (c *Config) GetDuration(key string) time.Duration {
    return cast.ToDuration(c.Get(key))
}

// Returns the value associated with the key as a slice of strings
func GetStringSlice(key string) []string { return c.GetStringSlice(key) }
func (c *Config) GetStringSlice(key string) []string {
    return cast.ToStringSlice(c.Get(key))
}

// Returns the value associated with the key as a map of interfaces
func GetStringMap(key string) map[string]interface{} { return c.GetStringMap(key) }
func (c *Config) GetStringMap(key string) map[string]interface{} {
    return cast.ToStringMap(c.Get(key))
}

// Returns the value associated with the key as a map of strings
func GetStringMapString(key string) map[string]string { return c.GetStringMapString(key) }
func (c *Config) GetStringMapString(key string) map[string]string {
    return cast.ToStringMapString(c.Get(key))
}

// Returns the value associated with the key as a map to a slice of strings.
func GetStringMapStringSlice(key string) map[string][]string { return c.GetStringMapStringSlice(key) }
func (c *Config) GetStringMapStringSlice(key string) map[string][]string {
    return cast.ToStringMapStringSlice(c.Get(key))
}

// Returns the size of the value associated with the given key
// in bytes.
func GetSizeInBytes(key string) uint { return c.GetSizeInBytes(key) }
func (c *Config) GetSizeInBytes(key string) uint {
    sizeStr := cast.ToString(c.Get(key))
    return parseSizeInBytes(sizeStr)
}

func UnmarshalKey(key string, rawVal interface{}) error {
    return c.UnmarshalKey(key, rawVal)
}
func (c *Config) UnmarshalKey(key string, rawVal interface{}) error {
    return mapstructure.Decode(c.Get(key), rawVal)
}

// Prints all configuration registries for debugging
// purposes.
func Debug() { c.Debug() }
func (c *Config) Debug() {
    fmt.Println("Aliases:")
    pretty.Println(c.aliases)
    // fmt.Println("Override:")
    // pretty.Println(c.override)
    // fmt.Println("PFlags")
    // pretty.Println(c.pflags)
    // fmt.Println("Env:")
    // pretty.Println(c.env)
    // fmt.Println("Key/Value Store:")
    // pretty.Println(c.kvstore)
    fmt.Println("Config:")
    pretty.Println(c.config)
    fmt.Println("Defaults:")
    pretty.Println(c.defaults)
}
