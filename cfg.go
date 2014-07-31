//Package cfg implements parsing and managing cfg configuration files
package cfg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

const trimChars = " \n\r\t"
const splitChar = "/"

type option struct {
	value   []string
	comment string
}

//This is a container of a cfg section. A full cfg file can be included in one *CFG and it's children
type CFG struct {
	root        *CFG
	inheritance *CFG
	parent      *CFG
	options     map[string]*option
	sections    map[string]*CFG
	order       []string
	comment     string
	lock        sync.Mutex
}

//Create a new *CFG
func NewCFG() (cfg *CFG) {
	cfg = new(CFG)
	cfg.options = make(map[string]*option)
	cfg.sections = make(map[string]*CFG)
	cfg.order = make([]string, 0)
	return
}

//Create a new *CFG loading the contents from the io.Reader
func NewCFGFromReader(r io.Reader) (cfg *CFG, err error) {
	cfg = NewCFG()
	err = cfg.LoadFromReader(r)
	return
}

func SplitPath(path string) []string {
	p := strings.Split(path, splitChar)
	current := 0
	for iP, iC := range p {
		if iC == "" {
			continue
		}
		if current < iP {
			p[current] = p[iP]
		}
		current++
	}
	return p[:current]
}

/* GFC funcs */
func (cfg *CFG) processSection(section_name string, remainder string, comment []string, inheritance_map map[*CFG]string) (*CFG, error) {
	if cfg.Exists(section_name) {
		return nil, errors.New(fmt.Sprintf("Section %s defined under %s is already defined", section_name, cfg.Path()))
	}
	subCfg := NewCFG()
	cfg.sections[section_name] = subCfg
	subCfg.root = cfg.root
	subCfg.parent = cfg
	subCfg.comment = strings.Join(comment, "\n")
	cfg.order = append(cfg.order, section_name)
	//Check if inheritance is defined
	remainder = strings.Trim(remainder, trimChars)
	if len(remainder) > 0 {
		if remainder[0] != '<' {
			return nil, errors.New(fmt.Sprintf("Expected inheriting section defined with '< section_name' but '%s' found", remainder))
		}
		inheritance_map[subCfg] = strings.Trim(remainder[1:], trimChars)
	}
	return subCfg, nil
}

func (cfg *CFG) processOption(parsedData []rune, opt_value string, comment []string) error {
	opt_value = strings.Trim(opt_value, trimChars)
	switch parsedData[len(parsedData)-1] {
	case '+':
		opt_name := strings.Trim(string(parsedData[:len(parsedData)-1]), trimChars)
		if opt_data, ok := cfg.options[opt_name]; ok {
			//Option is previously defined, so ok
			opt_data.value = append(opt_data.value, opt_value)
		} else {
			//Oops. Trying to append to a non existant option!
			return errors.New("Option " + opt_name + " was not previously defined")
		}
	default:
		opt_name := strings.Trim(string(parsedData), trimChars)
		if cfg.Exists(opt_name) {
			return errors.New(opt_name + " already exists")
		}
		//It is a new option
		cfg.options[opt_name] = &option{value: []string{opt_value},
			comment: strings.Join(comment, "\n")}
		cfg.order = append(cfg.order, opt_name)
	}
	return nil
}

//load the contents of a reader into this CFG. This method fails if something gets overwritten
func (cfg *CFG) LoadFromReader(r io.Reader) (err error) {
	return cfg.loadFromReader(bufio.NewReader(r), 0)
}

func (cfg *CFG) loadFromReader(source *bufio.Reader, line_counter uint32) (err error) {
	comment := make([]string, 0)
	line := ""
	parsedData := make([]rune, 0, 128)
	inheritance_map := make(map[*CFG]string)
	for ; err == nil; line, err = source.ReadString('\n') {
		line_counter++
		commentPos := strings.IndexRune(line, '#')
		if commentPos > -1 {
			comment = append(comment, strings.Trim(line[commentPos+1:], trimChars))
			line = strings.Trim(line[:commentPos], trimChars)
		}
		line = strings.Trim(line, trimChars)
		if len(line) == 0 {
			//Skip empty lines and lines starting with '#' (comments)
			continue
		}
	NextLineBreak:
		for lPos, lChar := range line {
			switch lChar {
			case '{':
				section_name := strings.Trim(string(parsedData), trimChars)
				var subCfg *CFG
				subCfg, err = cfg.processSection(section_name, line[lPos+1:], comment, inheritance_map)
				if err != nil {
					return errors.New(fmt.Sprintf("%s (line %v)", err.Error(), line_counter))
				}
				err = subCfg.loadFromReader(source, line_counter)
				if err != nil {
					return err
				}
			case '}':
				return nil
			case '=':
				err = cfg.processOption(parsedData, line[lPos+1:], comment)
				if err != nil {
					return errors.New(fmt.Sprintf("%s (line %v)", err.Error(), line_counter))
				}
				comment = comment[:0]
				parsedData = parsedData[:0]
				break NextLineBreak
			default:
				parsedData = append(parsedData, lChar)
			}

		}
	}
	if err == io.EOF {
		return nil
	}
	return err
}

//Return the path to this CFG from the root one
func (cfg *CFG) Path() string {
	lvls := 0
	for c := cfg; c.parent != nil; c = c.parent {
		lvls++
	}
	if lvls == 0 {
		return splitChar
	}
	path := make([]string, lvls)
	for i, me := lvls-1, cfg; i > -1; i, me = i-1, me.parent {
		for sName, sD := range me.parent.sections {
			if me == sD {
				path[i] = sName
				break
			}
		}
	}
	return strings.Join(path, splitChar)
}

//Get the root of the cfg
func (cfg *CFG) Root() *CFG {
	if cfg.root != nil {
		return cfg.root
	}
	return cfg
}

/* inner gets */
func (cfg *CFG) getString(path string, follow_inheritance bool, parent_lvl int) (*CFG, *option) {
	return cfg.get(strings.Split(path, splitChar), follow_inheritance, parent_lvl)
}

func (cfg *CFG) get(path []string, follow_inheritance bool, parent_lvl int) (*CFG, *option) {
	switch {
	case len(path) > 1+parent_lvl:
		if subCfg, ok := cfg.getSection(path[0], follow_inheritance); ok {
			return subCfg.get(path[1:], follow_inheritance, parent_lvl)
		}
	case len(path) == 1+parent_lvl:
		if sec, ok := cfg.getSection(path[0], follow_inheritance); ok {
			return sec, nil
		}
		if opt, ok := cfg.getOption(path[0], follow_inheritance); ok {
			return nil, opt
		}
	}
	return nil, nil
}

//Does a section or an option exist with this path starting from this section?
func (cfg *CFG) Exists(name string) bool {
	sec, opt := cfg.getString(name, true, 0)
	return sec != nil || opt != nil
}

//Does a section exist with this path starting from this section?
func (cfg *CFG) ExistsSection(name string) bool {
	sec, _ := cfg.getString(name, true, 0)
	return sec != nil
}

//Does an option exist with this path starting from this section?
func (cfg *CFG) ExistsOption(name string) bool {
	_, opt := cfg.getString(name, true, 0)
	return opt != nil
}

//Get section object under name
func (cfg *CFG) GetSection(name string) (*CFG, bool) {
	sec, _ := cfg.getString(name, true, 0)
	return sec, sec != nil
}

/* Real getters*/
func (cfg *CFG) getSection(name string, follow_inheritance bool) (*CFG, bool) {
	if sec, ok := cfg.sections[name]; ok {
		return sec, true
	}
	if follow_inheritance && cfg.inheritance != nil {
		return cfg.inheritance.getSection(name, true)
	}
	return nil, false
}

func (cfg *CFG) getOption(name string, follow_inheritance bool) (*option, bool) {
	if opt, ok := cfg.options[name]; ok {
		return opt, true
	}
	if follow_inheritance && cfg.inheritance != nil {
		return cfg.inheritance.getOption(name, true)
	}
	return nil, false
}

//Set an option value. This overwrites if it exists
func (cfg *CFG) SetOptionArray(name string, value []string, comment string) error {
	p := SplitPath(name)
	pcfg := cfg
	var opt *option
	switch len(p) {
	case 0:
		return errors.New("What is the name of the option?")
	case 1:
		opt = cfg.options[p[0]]
	default:
		pcfg, opt = cfg.get(p, false, 1)
		if pcfg == nil {
			return errors.New(fmt.Sprintf("Parent %s section does not exist", strings.Join(p[:len(p)-1], splitChar)))
		}
	}
	if opt == nil {
		opt = new(option)
		opt_name := p[len(p)-1]
		pcfg.options[opt_name] = opt
	}
	opt.comment = comment
	opt.value = value
	return nil
}

//Set option value as a string. Overwrites if option already exists
func (cfg *CFG) SetOption(name string, value string, comment string) error {
	return cfg.SetOptionArray(name, []string{value}, comment)
}

//Get option value as a string array
func (cfg *CFG) GetOptionArray(name string) ([]string, bool) {
	if _, opt := cfg.getString(name, true, 0); opt != nil {
		return opt.value, true
	}
	return nil, false
}

//Get option value as a string
func (cfg *CFG) GetOption(name string) (string, bool) {
	if _, opt := cfg.getString(name, true, 0); opt != nil {
		return strings.Join(opt.value, splitChar), true
	}
	return "", false
}

//Get option value if exists. If it doesn't or it cannot be retrieved for some reason, return default value
func (cfg *CFG) GetValue(name string, defaultValue string) string {
	if v, ok := cfg.GetOption(name); ok {
		return v
	}
	return defaultValue
}

//Get option value as string array if exists. If it doesn't or it cannot be retrieved for some reason, return default value
func (cfg *CFG) GetValueArray(name string, defaultValue []string) []string {
	if v, ok := cfg.GetOptionArray(name); ok {
		return v
	}
	return defaultValue
}