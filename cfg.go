// Package cfg atetempts to parse some config file (defaults to Go program calling it basename.)
// Ideally should be abstracted out more to parse any YAML, TOML based config file and return the struct.
package cfg

import (
    "bytes"
    "fmt"
    "io"
    "io/ioutil"
    "path/filepath"
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
    jww.UseTempLogFile("cfg")
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

    config    map[string]interface{}
    defaults  map[string]interface{}
    overrides map[string]interface{}
    aliases   map[string]string

    verbose        bool
    typeByDefValue bool
}

// Sets log file to the passed in paramter. Currently assumes the file is writable.
func SetLogFile(s string) { c.SetLogFile(s) }
func (c *Config) SetLogFile(s string) {
    if c.verbose {
        jww.SetLogThreshold(jww.LevelTrace)
        jww.SetStdoutThreshold(jww.LevelInfo)
    }
    jww.SetLogFile(s)
}

func SetVerbosity(v bool) { c.SetVerbosity(v) }
func (c *Config) SetVerbosity(v bool) {
    if v != true && v != false {
        jww.ERROR.Println("Incorrect value for verbosity provided.")
        c.verbose = false
    } else {
        c.verbose = v
    }
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
var SupportedExts []string = []string{"toml", "yaml", "yml"}

// Returns a properly initialized Config instance
func New() *Config {
    c := new(Config)
    c.keyDelm = "."
    c.configName = "config"
    c.config = make(map[string]interface{})
    c.defaults = make(map[string]interface{})
    c.overrides = make(map[string]interface{})
    c.aliases = make(map[string]string)
    c.typeByDefValue = false
    c.verbose = false

    return c
}

func Reset() {
    c = New()

    SupportedExts = []string{"toml", "yaml", "yml"}
}

// Explicitly sets the config file to be used.
func SetConfigFile(s string) { c.SetConfigFile(s) }
func (c *Config) SetConfigFile(s string) {
    if s != "" {
        c.configFile = s
    }
}

// Explicitly sets the config name to be used.
func SetConfigName(s string) { c.SetConfigName(s) }
func (c *Config) SetConfigName(s string) {
    if s != "" {
        c.configName = s
    }
}

// Explicitly sets the config file to be used.
func SetConfigType(s string) { c.SetConfigType(s) }
func (c *Config) SetConfigType(s string) {
    if s != "" {
        c.configType = s
    }
}

func (c *Config) getConfigType() string {
    if c.configType != "" {
        return c.configType
    }

    cf := c.getConfigFile()
    ext := filepath.Ext(cf)

    if len(ext) > 1 {
        return ext[1:]
    } else {
        return ""
    }
}

func (c *Config) getConfigFile() string {
    if c.configFile != "" {
        return c.configFile
    }

    cf, err := c.findConfigFile()
    if err != nil {
        return ""
    }

    c.configFile = cf
    return c.getConfigFile()
}

func (c *Config) searchInPath(in string) (filename string) {
    jww.DEBUG.Println("Searching for config in ", in)
    for _, ext := range SupportedExts {
        jww.DEBUG.Println("Checking for", filepath.Join(in, c.configName+"."+ext))
        if b, _ := exists(filepath.Join(in, c.configName+"."+ext)); b {
            jww.DEBUG.Println("Found: ", filepath.Join(in, c.configName+"."+ext))
            return filepath.Join(in, c.configName+"."+ext)
        }
    }

    return ""
}

// search all configPaths for any config file.
// Returns the first path that exists (and is a config file)
func (c *Config) findConfigFile() (string, error) {

    jww.INFO.Println("Searching for config in ", c.configPaths)

    for _, cp := range c.configPaths {
        file := c.searchInPath(cp)
        if file != "" {
            return file, nil
        }
    }
    return "", ConfigFileNotFoundError{c.configName, fmt.Sprintf("%s", c.configPaths)}
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

func Get(key string) interface{} { return c.Get(key) }
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

func Unmarshal(rawVal interface{}) error {
    return c.Unmarshal(rawVal)
}
func (c *Config) Unmarshal(rawVal interface{}) error {
    err := mapstructure.WeakDecode(c.AllSettings(), rawVal)

    if err != nil {
        return err
    }

    c.insensitiviseMaps()

    return nil
}

func (c *Config) find(key string) interface{} {
    var val interface{}
    var exists bool

    key = c.realKey(key)

    val, exists = c.overrides[key]
    if exists {
        jww.TRACE.Println(key, "found in overrides: ", val)
        return val
    }

    val, exists = c.config[key]
    if exists {
        jww.TRACE.Println(key, "found in config: ", val)
        return val
    }

    if strings.Contains(key, c.keyDelm) {
        path := strings.Split(key, c.keyDelm)

        source := c.find(path[0])
        if source != nil {
            if reflect.TypeOf(source).Kind() == reflect.Map {
                val := c.searchMap(cast.ToStringMap(source), path[1:])
                jww.TRACE.Println(key, "Found in nested config: ", val)
                return val
            }
        }
    }

    val, exists = c.defaults[key]
    if exists {
        jww.TRACE.Println(key, "found in defaults: ", val)
        return val
    }

    return nil
}

// Aliases provide another accessor for the same key.
// This enables one to change a name without breaking the application
func RegisterAlias(alias string, key string) { c.RegisterAlias(alias, key) }
func (c *Config) RegisterAlias(alias string, key string) {
    c.registerAlias(alias, strings.ToLower(key))
}

func (c *Config) registerAlias(alias string, key string) {
    alias = strings.ToLower(alias)
    if alias != key && alias != c.realKey(key) {
        _, exists := c.aliases[alias]

        if !exists {
            // if we alias something that exists in one of the maps to another
            // name, we'll never be able to get that value using the original
            // name, so move the config value to the new realkey.
            if val, ok := c.config[alias]; ok {
                delete(c.config, alias)
                c.config[key] = val
            }
            if val, ok := c.defaults[alias]; ok {
                delete(c.defaults, alias)
                c.defaults[key] = val
            }
            if val, ok := c.overrides[alias]; ok {
                delete(c.overrides, alias)
                c.overrides[key] = val
            }
            c.aliases[alias] = key
        }
    } else {
        jww.WARN.Println("Creating circular reference alias", alias, key, c.realKey(key))
    }
}

func IsSet(key string) bool { return c.IsSet(key) }
func (c *Config) IsSet(key string) bool {
    t := c.Get(key)
    return t != nil
}

func (c *Config) realKey(key string) string {
    newkey, exists := c.aliases[key]
    if exists {
        jww.DEBUG.Println("Alias", key, "to", newkey)
        return c.realKey(newkey)
    } else {
        return key
    }
}

func InConfig(key string) bool { return c.InConfig(key) }
func (c *Config) InConfig(key string) bool {
    key = c.realKey(key)

    _, exists := c.config[key]
    return exists
}

func SetDefault(key string, value interface{}) { c.SetDefault(key, value) }
func (c *Config) SetDefault(key string, value interface{}) {
    key = c.realKey(strings.ToLower(key))
    c.defaults[key] = value
}

func Set(key string, value interface{}) { c.Set(key, value) }
func (c *Config) Set(key string, value interface{}) {
    key = c.realKey(strings.ToLower(key))
    c.overrides[key] = value
}

func ReadInConfig() error { return c.ReadInConfig() }
func (c *Config) ReadInConfig() error {
    jww.INFO.Println("Attempting to read in config file")
    if !stringInSlice(c.getConfigType(), SupportedExts) {
        return UnsupportedConfigError(c.getConfigType())
    }

    file, err := ioutil.ReadFile(c.getConfigFile())
    if err != nil {
        return err
    }

    c.config = make(map[string]interface{})

    return c.unmarshalReader(bytes.NewReader(file), c.config)
}

func unmarshalReader(in io.Reader, v map[string]interface{}) error {
    return c.unmarshalReader(in, v)
}
func (c *Config) unmarshalReader(in io.Reader, v map[string]interface{}) error {
    return unmarshallConfigReader(in, v, c.getConfigType())
}

func (c *Config) insensitiviseMaps() {
    insensitiviseMap(c.config)
    insensitiviseMap(c.defaults)
    insensitiviseMap(c.overrides)
}

func AllKeys() []string { return c.AllKeys() }
func (c *Config) AllKeys() []string {
    m := map[string]struct{}{}

    for key := range c.defaults {
        m[key] = struct{}{}
    }

    for key := range c.config {
        m[key] = struct{}{}
    }

    for key := range c.overrides {
        m[key] = struct{}{}
    }

    a := []string{}
    for x := range m {
        a = append(a, x)
    }

    return a
}

func AllSettings() map[string]interface{} { return c.AllSettings() }
func (c *Config) AllSettings() map[string]interface{} {
    m := map[string]interface{}{}
    for _, x := range c.AllKeys() {
        m[x] = c.Get(x)
    }

    return m
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
