//Package cfg implements parsing and managing cfg configuration files
package cfg

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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
	inheritance *CFG
	parent      *CFG
	options     map[string]*option
	sections    map[string]*CFG
	order       []string
	comment     string
	lock        *sync.Mutex
}

//Create a new *CFG
func NewCFG() (cfg *CFG) {
	cfg = newCFG()
	cfg.lock = new(sync.Mutex)
	return
}

func newCFG() (cfg *CFG) {
	cfg = new(CFG)
	cfg.options = make(map[string]*option)
	cfg.sections = make(map[string]*CFG)
	cfg.order = make([]string, 0)
	return
}

//Create a new *CFG loading the contents from a filename
func NewCFGFromFile(filename string) (cfg *CFG, err error) {
	fi, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fi.Close()
	return NewCFGFromReader(fi)
}

//Create a new *CFG loading the contents from a string
func NewCFGFromString(data string) (*CFG, error) {
	return NewCFGFromReader(strings.NewReader(data))
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

//Stringer interface
func (cfg *CFG) String() string {
	var b bytes.Buffer
	err := cfg.DumpToWriter(&b)
	if err == nil {
		return b.String()
	}
	return ""
}

//Dump
func (cfg *CFG) DumpToWriter(w io.Writer) error {
	return cfg.dumpToWriter(w, 0)
}

func (cfg *CFG) dumpCommentToWriter(w io.Writer, comment string, indent string) error {
	if comment == "" {
		return nil
	}
	for _, cl := range strings.Split(comment, "\n") {
		if len(cl) > 0 {
			line := indent + "#" + cl + "\n"
			if _, err := w.Write([]byte(line)); err != nil {
				return err
			}
		}
	}
	return nil

}

func (cfg *CFG) dumpToWriter(w io.Writer, indent_lvl int) error {
	indent := strings.Repeat("\t", indent_lvl)
	var line string
	for _, name := range cfg.order {
		//Dump the section
		if sec, ok := cfg.sections[name]; ok {
			if err := cfg.dumpCommentToWriter(w, sec.comment, indent); err != nil {
				return err
			}
			line = indent + name + " {"
			if sec.inheritance != nil {
				line += "< " + sec.inheritance.Path()
			}
			if _, err := w.Write([]byte(line + "\n")); err != nil {
				return err
			}
			if err := sec.dumpToWriter(w, indent_lvl+1); err != nil {
				return err
			}
			line = indent + "}" + "\n"
			if _, err := w.Write([]byte(line)); err != nil {
				return err
			}
		}
		if opt, ok := cfg.options[name]; ok {
			if err := cfg.dumpCommentToWriter(w, opt.comment, indent); err != nil {
				return err
			}
			for nV, val := range opt.value {
				if nV == 0 {
					line = indent + name + " = " + val + "\n"
				} else {
					line = indent + name + " += " + val + "\n"
				}
				if _, err := w.Write([]byte(line)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

//load the contents of a reader into this CFG. This method fails if something gets overwritten
func (cfg *CFG) LoadFromReader(r io.Reader) (err error) {
	inheritance_map := make(map[*CFG]string)
	err = cfg.loadFromReader(bufio.NewReader(r), 0, inheritance_map)
	if err != nil {
		return
	}
	cfg.resetInheritance()
	for child, inheritance := range inheritance_map {
		if err = child.SetInheritance(inheritance); err != nil {
			return
		}
	}
	return
}

//Reset all inheritance pointers for this cfg and child ones
func (cfg *CFG) resetInheritance() {
	cfg.inheritance = nil
	for _, subCFG := range cfg.sections {
		subCFG.resetInheritance()
	}
}

//Define an inheritance section for this cfg. That means that any time that an option or section is retrieved, if this cfg does not have it it will check the inheritance one
func (cfg *CFG) SetInheritance(inheritance string) error {
	incfg, _ := cfg.Root().getString(inheritance, false, 0)
	myPath := cfg.Path()
	if incfg == nil {
		return errors.New(fmt.Sprintf("Inheritance section %s for section %s does not exist", inheritance, myPath))
	}
	cfg.inheritance = incfg
	path := []string{myPath, incfg.Path()}
	search := incfg.inheritance
	for search != nil {
		path = append(path, search.Path())
		if search == cfg {
			return errors.New("Circular inheritance loop found: " + strings.Join(path, " < "))
		}
		search = search.inheritance
	}
	return nil
}

func (cfg *CFG) processSection(section_name string, remainder string, comment []string, inheritance_map map[*CFG]string) (*CFG, error) {
	if ocfg, opt := cfg.getString(section_name, false, 0); ocfg != nil || opt != nil {
		return nil, errors.New(fmt.Sprintf("Section %s defined under %s is already defined", section_name, cfg.Path()))
	}
	subCfg, err := cfg.CreateSection(section_name, strings.Join(comment, "\n"))
	if err != nil {
		return subCfg, err
	}
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
		if _, opt := cfg.getString(opt_name, false, 0); opt != nil {
			//Option is previously defined, so ok
			opt.value = append(opt.value, opt_value)
		} else {
			//Oops. Trying to append to a non existant option!
			return errors.New("Option " + opt_name + " was not previously defined")
		}
	default:
		opt_name := strings.Trim(string(parsedData), trimChars)
		if sec, opt := cfg.getString(opt_name, false, 0); sec != nil || opt != nil {
			return errors.New(opt_name + " already exists")
		}
		return cfg.SetOptionArray(opt_name, []string{opt_value}, strings.Join(comment, splitChar))
	}
	return nil
}

func (cfg *CFG) loadFromReader(source *bufio.Reader, line_counter uint32, inheritance_map map[*CFG]string) (err error) {
	comment := make([]string, 0)
	line := ""
	parsedData := make([]rune, 0, 128)
	for err == nil {
		line, err = source.ReadString('\n')
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
				err = subCfg.loadFromReader(source, line_counter, inheritance_map)
				if err != nil {
					return err
				}
				comment = comment[:0]
				parsedData = parsedData[:0]
				break NextLineBreak
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
	root := cfg
	for root.parent != nil {
		root = root.parent
	}
	return root
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

//Creates a section.Does not create all the intermediate ones and does not overwrite if there's one already present
func (cfg *CFG) CreateSection(name string, comment string) (*CFG, error) {
	cfg.Root().lock.Lock()
	defer cfg.Root().lock.Unlock()
	p := SplitPath(name)
	var parentCfg *CFG
	switch len(p) {
	case 0:
		return nil, errors.New("What's the name of the section?")
	case 1:
		parentCfg = cfg
	default:
		parentCfg, _ := cfg.get(p, false, 1)
		if parentCfg == nil {
			return nil, errors.New("Parent section for " + strings.Join(p, "\n") + " does not exist")
		}
	}
	if _, ok := parentCfg.sections[p[0]]; ok {
		return nil, errors.New("Section " + p[0] + " already exists")
	}
	section_name := p[len(p)-1]
	subCfg := newCFG()
	parentCfg.sections[section_name] = subCfg
	parentCfg.order = append(parentCfg.order, section_name)
	subCfg.parent = parentCfg
	subCfg.comment = comment
	return subCfg, nil
}

//Set an option value. This overwrites if it exists
func (cfg *CFG) SetOptionArray(name string, value []string, comment string) error {
	cfg.Root().lock.Lock()
	defer cfg.Root().lock.Unlock()
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
		pcfg.order = append(cfg.order, opt_name)
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

//Clone a CFG. If it's not the root one it will just dup from that section downwards. Upper inheritance links will still point to their original sources. Lower ones will point to the new created sections
func (cfg *CFG) Clone() (dup *CFG, err error) {
	dup = newCFG()
	if cfg.parent == nil {
		dup.lock = new(sync.Mutex)
	} else {
		dup.parent = cfg.parent
	}
	var buf bytes.Buffer
	if err = cfg.DumpToWriter(&buf); err != nil {
		return
	}
	err = dup.LoadFromReader(&buf)
	return
}

//Are the two CFGs equal (including comments)
func (cfg *CFG) RealEqual(other *CFG) bool {
	return cfg.equal(other, true)
}

//Are the two CFGs equal (NOT including comments)
func (cfg *CFG) Equal(other *CFG) bool {
	return cfg.equal(other, false)
}

func (cfg *CFG) equal(other *CFG, with_comments bool) bool {
	if with_comments && cfg.comment != other.comment {
		return false
	}
	if len(cfg.order) != len(other.order) {
		return false
	}
	switch {
	case cfg.inheritance != nil:
		if other.inheritance == nil {
			return false
		}
		if cfg.inheritance.Path() != other.inheritance.Path() {
			return false
		}
	default:
		if other.inheritance != nil {
			return false
		}
	}
	for iPos, name := range cfg.order {
		if other.order[iPos] != name {
			return false
		}
		if sec, ok := cfg.sections[name]; ok {
			if other_sec, ok2 := other.sections[name]; ok2 {
				if !sec.equal(other_sec, with_comments) {
					return false
				}
			} else {
				return false
			}
		}
		if opt, ok := cfg.options[name]; ok {
			if other_opt, ok2 := other.options[name]; ok2 {
				if with_comments && opt.comment != other_opt.comment {
					return false
				}
				if len(opt.value) != len(other_opt.value) {
					return false
				}
				for vPos, val := range opt.value {
					if other_opt.value[vPos] != val {
						return false
					}
				}
			} else {
				return false
			}
		}
	}
	return true
}