// Copyright © 2016-2017 Martin Tournoij
// See the bottom of this file for the full copyright.

// Package sconfig is a simple yet functional configuration file parser.
//
// See the README.markdown for an introduction.
package sconfig // import "arp242.net/sconfig"

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
)

var (
	// typeHandlers are all the registered type handlers.
	//
	// The key is the name of the type, the value the list of handler functions
	// to run.
	typeHandlers = make(map[string][]TypeHandler)
)

// TypeHandler takes the field to set and the value to set it to. It is expected
// to return the value to set it to.
type TypeHandler func([]string) (interface{}, error)

// Handler functions can be used to run special code for a field. The function
// takes the unprocessed line split by whitespace and with the option name
// removed.
type Handler func([]string) error

// Handlers can be used to run special code for a field. The map key is the name
// of the field in the struct.
type Handlers map[string]Handler

// RegisterType sets the type handler functions for a type. Existing handlers
// are always overridden (it doesn't add to the list!)
//
// The handlers are chained; the return value is passed to the next one. The
// chain is stopped if one handler returns a non-nil error. This is particularly
// useful for validation (see ValidateSingleValue() and ValidateValueLimit() for
// examples).
func RegisterType(typ string, fun ...TypeHandler) {
	typeHandlers[typ] = fun
}

// readFile will read a file, strip comments, and collapse indents. This also
// deals with the special "source" command.
//
// The return value is an nested slice where the first item is the original line
// number and the second is the parsed line; for example:
//
//     [][]string{
//         []string{3, "key value"},
//         []string{9, "key2 value1 value2"},
//     }
//
// The line numbers can be used later to give more informative error messages.
//
// The input must be utf-8 encoded; other encodings are not supported.
func readFile(file string) (lines [][]string, err error) {
	fp, err := os.Open(file)
	if err != nil {
		return lines, err
	}
	defer func() { _ = fp.Close() }()

	i := 0
	no := 0
	for scanner := bufio.NewScanner(fp); scanner.Scan(); {
		no++
		line := scanner.Text()

		isIndented := len(line) > 0 && unicode.IsSpace(rune(line[0]))
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || line[0] == '#' {
			continue
		}

		line = collapseWhitespace(removeComments(line))

		if isIndented {
			if i == 0 {
				return lines, fmt.Errorf("first line can't be indented")
			}
			// Append to previous line; don't increment i since there may be
			// more indented lines.
			lines[i-1][1] += " " + line
		} else {
			// Source command
			if strings.HasPrefix(line, "source ") {
				sourced, err := readFile(line[7:])
				if err != nil {
					return nil, err
				}
				lines = append(lines, sourced...)
			} else {
				lines = append(lines, []string{fmt.Sprintf("%d", no), line})
			}
			i++
		}
	}

	return lines, nil
}

func removeComments(line string) string {
	prevcmt := 0
	for {
		cmt := strings.Index(line[prevcmt:], "#")
		if cmt < 0 {
			break
		}

		cmt += prevcmt
		prevcmt = cmt

		// Allow escaping # with \#
		if line[cmt-1] == '\\' {
			line = line[:cmt-1] + line[cmt:]
		} else {
			// Found comment
			line = line[:cmt]
			break
		}
	}

	return line
}

func collapseWhitespace(line string) string {
	nl := ""
	prevSpace := false
	for i, char := range line {
		switch {
		case char == '\\':
			// \ is escaped with \: "\\"
			if line[i-1] == '\\' {
				nl += `\`
			}
		case unicode.IsSpace(char):
			if prevSpace {
				// Escaped with \: "\ "
				if line[i-1] == '\\' {
					nl += string(char)
				}
			} else {
				prevSpace = true
				if i != len(line)-1 {
					nl += " "
				}
			}
		default:
			nl += string(char)
			prevSpace = false
		}
	}

	return nl
}

// MustParse behaves like Parse, but panics if there is an error.
func MustParse(c interface{}, file string, handlers Handlers) {
	err := Parse(c, file, handlers)
	if err != nil {
		panic(err)
	}
}

// DontPanic indicates that Parse should never panic(). It's sometimes useful to
// disable this when you want a full stack trace.
var DontPanic = true

// Parse will reads file from disk and populates the given config struct.
//
// The Handlers map can be given to customize the behaviour for individual
// configuration keys. This will override the type handler (if any).
//
// The function is expected to set any settings on the struct; for example:
//
//  Parse(&config, "config", Handlers{
//      "Bool": func(line []string) error {
//          if line[0] == "yup" {
//              config.Bool = true
//          }
//          return nil
//       },
//  })
//
// Returned errors will abort parsing and set the error as the return value for
// Parse().
func Parse(config interface{}, file string, handlers Handlers) (returnErr error) {
	// Recover from panics; return them as errors!
	// TODO: This loses the stack though...
	defer func() {
		if DontPanic {
			if rec := recover(); rec != nil {
				switch recType := rec.(type) {
				case error:
					returnErr = recType
				default:
					panic(rec)
				}
			}
		}
	}()

	lines, err := readFile(file)
	if err != nil {
		return err
	}

	values := getValues(config)

	// Get list of rule names from tags
	for _, line := range lines {
		// Split by spaces
		v := strings.Split(line[1], " ")

		var field reflect.Value
		var fieldName string

		switch values.Kind() {

		// TODO: Only support map[string][]string atm.
		case reflect.Map:
			fieldName = v[0]
			mapKey := reflect.ValueOf(v[0]).Convert(reflect.TypeOf(fieldName))
			values.SetMapIndex(mapKey, reflect.ValueOf(v[1:]))

			continue

		case reflect.Struct:
			// Infer the field name from the key
			var err error
			fieldName, err = fieldNameFromKey(v[0], values)
			if err != nil {
				return fmterr(file, line[0], v[0], err)
			}
			field = values.FieldByName(fieldName)

		default:
			return fmt.Errorf("unknown type: %v", values.Kind())
		}

		// Use the handler if it exists
		if has, err := setFromHandler(fieldName, v[1:], handlers); has {
			if err != nil {
				return fmterr(file, line[0], v[0], err)
			}
			continue
		}

		// Set from type handler
		if has, err := setFromTypeHandler(&field, v[1:]); has {
			if err != nil {
				return fmterr(file, line[0], v[0], err)
			}
			continue
		}

		// Give up :-(
		return fmterr(file, line[0], v[0], fmt.Errorf(
			"don't know how to set fields of the type %s",
			field.Type().String()))
	}

	return returnErr // Can be set by defer
}

func getValues(c interface{}) reflect.Value {
	// Make sure we give a sane error here when accidentally passing in a
	// non-pointer, since the default is not all that helpful:
	//     panic: reflect: call of reflect.Value.Elem on struct Value
	defer func() {
		err := recover()
		if err != nil {
			switch err.(type) {
			case *reflect.ValueError:
				panic(fmt.Errorf(
					"unable to get values of the config struct (did you pass it as a pointer?): %v",
					err))
			default:
				panic(err)
			}
		}
	}()
	return reflect.ValueOf(c).Elem()
}

func fmterr(file, line, key string, err error) error {
	return fmt.Errorf("%v line %v: error parsing %s: %v",
		file, line, key, err)
}

func fieldNameFromKey(key string, values reflect.Value) (string, error) {
	fieldName := inflect.camelize(key)

	// This list is from golint
	acr := []string{"Api", "Ascii", "Cpu", "Css", "Dns", "Eof", "Guid", "Html",
		"Https", "Http", "Id", "Ip", "Json", "Lhs", "Qps", "Ram", "Rhs",
		"Rpc", "Sla", "Smtp", "Sql", "Ssh", "Tcp", "Tls", "Ttl", "Udp",
		"Ui", "Uid", "Uuid", "Uri", "Url", "Utf8", "Vm", "Xml", "Xsrf",
		"Xss"}
	for _, a := range acr {
		fieldName = strings.Replace(fieldName, a, strings.ToUpper(a), -1)
	}

	field := values.FieldByName(fieldName)
	if !field.CanAddr() {
		// Check plural version too; we're not too fussy
		fieldNamePlural := inflect.togglePlural(fieldName)
		field = values.FieldByName(fieldNamePlural)
		if !field.CanAddr() {
			return "", fmt.Errorf("unknown option (field %s or %s is missing)",
				fieldName, fieldNamePlural)
		}
		fieldName = fieldNamePlural
	}

	return fieldName, nil
}

func setFromHandler(fieldName string, values []string, handlers Handlers) (bool, error) {
	if handlers == nil {
		return false, nil
	}

	handler, has := handlers[fieldName]
	if !has {
		return false, nil
	}

	err := handler(values)
	if err != nil {
		return true, fmt.Errorf("%v (from handler)", err)
	}

	return true, nil
}

func setFromTypeHandler(field *reflect.Value, value []string) (bool, error) {
	handler, has := typeHandlers[field.Type().String()]
	if !has {
		return false, nil
	}

	var (
		v   interface{}
		err error
	)
	for _, h := range handler {
		v, err = h(value)
		if err != nil {
			return true, err
		}
	}

	val := reflect.ValueOf(v)
	if field.Kind() == reflect.Slice {
		val = reflect.AppendSlice(*field, val)
	}
	field.Set(val)
	return true, nil
}

// FindConfig tries to find a configuration file at the usual locations.
//
// The following paths are checked (in this order):
//
//   $XDG_CONFIG/$file
//   $HOME/.$file
//   /etc/$file
//   /usr/local/etc/$file
//   /usr/pkg/etc/$file
//   ./$file
//
// The default for $XDG_CONFIG if unset is $HOME/.config
func FindConfig(file string) string {
	file = strings.TrimLeft(file, "/")

	locations := []string{}
	xdg := os.Getenv("XDG_CONFIG")
	if xdg != "" {
		locations = append(locations, filepath.Join(xdg, file))
	}
	if home := os.Getenv("HOME"); home != "" {
		if xdg == "" {
			locations = append(locations, filepath.Join(
				os.Getenv("HOME"), "/.config/", file))
		}
		locations = append(locations, home+"/."+file)
	}

	locations = append(locations, []string{
		"/etc/" + file,
		"/usr/local/etc/" + file,
		"/usr/pkg/etc/" + file,
		"./" + file,
	}...)

	for _, l := range locations {
		if _, err := os.Stat(l); err == nil {
			return l
		}
	}

	return ""
}

// The MIT License (MIT)
//
// Copyright © 2016-2017 Martin Tournoij
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// The software is provided "as is", without warranty of any kind, express or
// implied, including but not limited to the warranties of merchantability,
// fitness for a particular purpose and noninfringement. In no event shall the
// authors or copyright holders be liable for any claim, damages or other
// liability, whether in an action of contract, tort or otherwise, arising
// from, out of or in connection with the software or the use or other dealings
// in the software.
