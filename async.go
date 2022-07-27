package async

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"time"

	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
)

type methodID struct {
	rType reflect.Type
	name  string
}

type methodInfo struct {
	jobName           string
	receivesContext   bool
	returnsError      bool
	marshalStructType reflect.Type
	paramTypes        []string
}

type Executor struct {
	methods map[methodID]methodInfo
}

func NewExecutor() *Executor {
	return &Executor{
		methods: map[methodID]methodInfo{},
	}
}

// Register returns a handler to call a method on the receiver.
// The handler should be registered in jobs processor.
//
// Method is a func that takes receiver as the first input parameter.
// The next input parameter may optionally be context.Context.
// It optionally may return an error (or a type implementing error interface).
//
// Receiver should have the same type as the first parameter of the method.
// I.e. if method is declared on a pointer, receiver should be a pointer.
//
// You can't register the same jobName twice.
// You can't register the same method twice.
func (e *Executor) Register( // nolint:gocognit // TODO: simplify
	jobName string,
	receiver interface{},
	method interface{},
) (func(context.Context, json.RawMessage) error, error) {
	receiverVal := reflect.ValueOf(receiver)
	methodVal := reflect.ValueOf(method)
	methodType := methodVal.Type()

	info, err := e.validateAndGetMethodInfo(jobName, receiverVal.Type(), methodType)
	if err != nil {
		return nil, errors.WithMessagef(err, "invalid method")
	}

	id := e.methodID(methodVal)

	if _, ok := e.methods[id]; ok {
		return nil, errors.Errorf("method already registered %s %s", id.name, methodType)
	}
	for _, i := range e.methods {
		if jobName == i.jobName {
			return nil, errors.Errorf("jobName already registered %q", jobName)
		}
	}

	e.methods[id] = info

	return func(ctx context.Context, marshaledArgs json.RawMessage) error {
		marshalStructPtr := reflect.New(info.marshalStructType)
		marshalStruct := marshalStructPtr.Elem()

		errJSON := json.Unmarshal(marshaledArgs, marshalStructPtr.Interface())
		if errJSON != nil {
			return errors.Wrapf(errJSON, "can't unmarshal arguments")
		}

		args := make([]reflect.Value, 0, methodType.NumIn())
		args = append(args, receiverVal)
		if info.receivesContext {
			args = append(args, reflect.ValueOf(ctx))
		}

		for i, expectedType := range info.paramTypes {
			actualType := marshalStruct.Field(1 + i*2).String()
			if actualType == "" {
				return errors.Errorf("args[%d]: missing value (type %s)", i, expectedType)
			}
			if expectedType != actualType {
				return errors.Errorf("args[%d]: wrong type: %s expected, received %s", i, expectedType, actualType)
			}

			args = append(args, marshalStruct.Field(i*2)) // nolint:gomnd // even numbers
		}

		res := methodVal.Call(args)

		if !info.returnsError {
			return nil
		}
		if res[0].IsNil() {
			return nil
		}
		return (res[0].Interface()).(error)
	}, nil
}

// Prepare returns jobName and marshaled args to be enqueued.
//
// Arguments are serialized to be saved inside jobs storage.
// It is a bad practice to use domain types as a transport ones directly.
// So library limits allowed types as much as possible.
// However, it still may be error-prone, see test "using iota is dangerous".
func (e *Executor) Prepare(method interface{}, args ...interface{}) (string, json.RawMessage, error) {
	methodType := reflect.TypeOf(method)
	if methodType.Kind() != reflect.Func {
		return "", nil, errors.Errorf("func expected, received %s", methodType)
	}

	id := e.methodID(reflect.ValueOf(method))

	info, ok := e.methods[id]
	if !ok {
		return "", nil, errors.Errorf("not registered method %s %s", id.name, methodType)
	}

	expectedLen := len(info.paramTypes)
	if len(args) != expectedLen {
		return "", nil, errors.Errorf("expected %d args, received %d", expectedLen, len(args))
	}

	marshalStructPtr := reflect.New(info.marshalStructType)
	marshalStruct := marshalStructPtr.Elem()
	offset := methodType.NumIn() - expectedLen

	for i := 0; i < expectedLen; i++ {
		eType := methodType.In(offset + i)
		aType := reflect.TypeOf(args[i])
		// TODO: [good idea?] convert nil to zero value of expected type
		//                    type MyTypeWithIgnoredValue string
		// TODO: [good idea?] compare kinds and do conversion if matches (may be useful if type MyInt int is used, but gust )
		if aType != eType {
			return "", nil, errors.Errorf("args[%d]: expected %s, received %s", i, eType, aType)
		}

		marshalStruct.Field(i * 2).Set(reflect.ValueOf(args[i])) // nolint:gomnd // even numbers
		marshalStruct.Field(1 + i*2).SetString(info.paramTypes[i])
	}

	data, err := json.Marshal(marshalStructPtr.Interface())
	if err != nil {
		return "", nil, errors.Wrapf(err, "can't marshal arguments")
	}

	return info.jobName, data, nil
}

func (e *Executor) methodID(methodVal reflect.Value) methodID {
	return methodID{
		rType: methodVal.Type(),
		// Hope this will not change in the future versions of go
		name: runtime.FuncForPC(methodVal.Pointer()).Name(),
	}
}

func (e *Executor) validateAndGetMethodInfo(
	jobName string,
	receiver reflect.Type,
	method reflect.Type,
) (methodInfo, error) {
	if method.Kind() != reflect.Func {
		return methodInfo{}, errors.Errorf("func expected, received %s", method)
	}
	if method.IsVariadic() {
		// TODO: support in the future?
		return methodInfo{}, errors.Errorf("variadic methods not supported %s", method)
	}
	if method.NumIn() < 1 {
		return methodInfo{}, errors.Errorf("is not a method (there is no receiver) %s", method)
	}
	if method.In(0) != receiver {
		return methodInfo{}, errors.Errorf("wrong receiver: %s expected, received %s", method.In(0), receiver)
	}

	receivesContext := false
	returnsError := false
	marshalFields := make([]reflect.StructField, 0, 2*method.NumIn()) // nolint:gomnd // param encoded with 2 fields
	paramTypes := make([]string, 0, method.NumIn())
	offset := 1

	for i := 1; i < method.NumIn(); i++ { // 1 is to skip receiver
		aType := method.In(i)

		if i == 1 && aType == reflect.TypeOf(new(context.Context)).Elem() {
			receivesContext = true
			offset++
			continue
		}

		idx := i - offset

		paramTypeString, err := e.validateAndGetParameterTypeString(aType)
		if err != nil {
			return methodInfo{}, errors.Wrapf(err, "args[%d]", idx)
		}

		marshalFields = append(marshalFields, reflect.StructField{
			Name: fmt.Sprintf("Arg%d", idx),
			Type: aType,
			Tag:  reflect.StructTag(fmt.Sprintf(`json:"arg%d"`, idx)),
		})
		marshalFields = append(marshalFields, reflect.StructField{
			Name: fmt.Sprintf("Type%d", idx),
			Type: reflect.TypeOf(""),
			Tag:  reflect.StructTag(fmt.Sprintf(`json:"type%d"`, idx)),
		})
		paramTypes = append(paramTypes, paramTypeString)
	}

	if method.NumOut() > 0 {
		if !method.Out(0).Implements(reflect.TypeOf(new(error)).Elem()) {
			return methodInfo{}, errors.New("if method returns something, it should implement error interface")
		}
		if method.NumOut() > 1 {
			return methodInfo{}, errors.Errorf("method should return at most 1 value, but returns %d", method.NumOut())
		}
		returnsError = true
	}

	return methodInfo{
		jobName:           jobName,
		receivesContext:   receivesContext,
		returnsError:      returnsError,
		marshalStructType: reflect.StructOf(marshalFields),
		paramTypes:        paramTypes,
	}, nil
}

func (e *Executor) validateAndGetParameterTypeString(aType reflect.Type) (string, error) {
	switch aType.Kind() {
	// basic types

	case reflect.String:
		fallthrough
	case reflect.Bool:
		fallthrough
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fallthrough
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fallthrough
	case reflect.Float32, reflect.Float64:
		fallthrough
	case reflect.Complex64, reflect.Complex128:
		return aType.Kind().String(), nil

	// structs

	case reflect.Struct:
		switch aType {
		case reflect.TypeOf(time.Time{}):
			return "builtin time.Time", nil
		case reflect.TypeOf(decimal.Decimal{}):
			return "github.com/shopspring/decimal.Decimal", nil
		default:
			if aType.NumField() == 0 {
				return "empty struct", nil
			}
			// TODO: add more allowed types
			return "", errors.Errorf("unsupported type %s", aType)
		}

	// types that may be supported in the future

	case reflect.Slice, reflect.Array, reflect.Map, reflect.Interface, reflect.Ptr:
		return "", errors.Errorf("unsupported type %s", aType)

	// unsupported

	case reflect.Chan, reflect.Func, reflect.Invalid, reflect.Uintptr, reflect.UnsafePointer:
		return "", errors.Errorf("unsupported type %s", aType)
	default:
		return "", errors.Errorf("[bug] unsupported type %s", aType)
	}
}
