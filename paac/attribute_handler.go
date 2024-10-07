package paac

import (
	"fmt"
	"log"
	"maps"
	"sync"
)

// A [GenericAttributeHandler] can be used to maintain a mapping of keys to
// attribute sets. These sets will consist of the same attributes across keys, and
// methods are provided to add or remove attributes. Any attributes which are not
// explicitly initiated will be initiated with their default value, which must
// be set when the handler is initiated and whenever a new attribute is added.
// Default values may also be functions. See [NewGenericAttributeHandler] and
// [GenericAttributeHandler.NewAttribute]
//
// NOTE: The handler *should* be thread-safe

type GenericAttributeHandler struct {
	// Defines the set of attributes stored for each key and their default value
	DefaultValues map[string]any
	// Mapping from keys to attribute sets
	Attributes map[string]map[string]any
	// Lock since we may access attributes concurrently
	mu sync.RWMutex
	// Signals any running routine to return
	closed chan bool

	Logger *log.Logger
}

// Contains an attribute map for the given key if ok.
// If ok is false, the key was not valid
type GenericAttributes struct {
	Key        string
	Attributes map[string]any
	Ok         bool
}

// Creates a new [GenericAttributeHandler]
// A map of starting attributes and their respective default defaultValues
// can be passed as argument. If the default value is a function of type
//
//	func(string)any
//
// instead, the function is called with the key as argument and the return value is used.
func NewGenericAttributeHandler(logger *log.Logger, defaultValues map[string]any) *GenericAttributeHandler {
	if defaultValues == nil {
		defaultValues = make(map[string]any)
	}
	return &GenericAttributeHandler{
		DefaultValues: defaultValues,
		Attributes:    make(map[string]map[string]any),
		Logger:        logger,
		closed:        make(chan bool),
	}
}

// Adds a new attribute and default value. This attribute's default value is added to
// all existing keys. If the default value is a function of type
//
//	func(string)any
//
// instead, the function is called with the key as argument and the return value is used.
func (h *GenericAttributeHandler) NewAttribute(key string, defaultValue any) error {
	var err error
	h.mu.Lock()
	defer h.mu.Unlock()
	for k := range h.DefaultValues {
		if k == key {
			err = fmt.Errorf("Attribute \"%v\" already exists. Default value: %v\n", key, defaultValue)
			return err
		}
	}

	h.DefaultValues[key] = defaultValue

	defaultFunc, ok := defaultValue.(func(string) any)
	if ok {
		for k, attrMap := range h.Attributes {
			attrMap[key] = defaultFunc(k)
		}
	} else {
		for _, attrMap := range h.Attributes {
			attrMap[key] = defaultValue
		}
	}
	if LogLevel >= 4 {
		h.Logger.Printf("Added new generic attribute: key=%v, default=%v\n", key, defaultValue)
		h.Logger.Println("New attribute set:", h.DefaultValues)
	}
	return err
}

// Removes the given attribute from all stored keys and as default value.
// If there is no such element, RemoveAttribute is a no-op.
func (h *GenericAttributeHandler) RemoveAttribute(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.DefaultValues[key]; !ok {
		return
	}
	delete(h.DefaultValues, key)
	for _, v := range h.Attributes {
		delete(v, key)
	}
	if LogLevel >= 4 {
		h.Logger.Printf("Removed generic attribute: key=%v\n", key)
		h.Logger.Println("New attribute set:", h.DefaultValues)
	}
}

// Stores the given attributes with the given key.
// All attributes must be valid attributes specified in [GenericAttributeHandler.DefaultValues].
// Any missing attributes are instantiated with their respective default values.
func (h *GenericAttributeHandler) Put(key string, attributes map[string]any) (err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k := range attributes {
		if _, ok := h.DefaultValues[k]; !ok {
			return fmt.Errorf("Provided attribute map includes invalid key \"%v\". "+
				"Please add the key as a new attribute first.\n", k)
		}
	}
	v := make(map[string]any)
	maps.Copy(v, h.DefaultValues)
	for k, attr := range v {
		if f, ok := attr.(func(string) any); ok {
			v[k] = f(k)
		}
	}
	maps.Copy(v, attributes)
	h.Attributes[key] = v
	if LogLevel >= 5 {
		h.Logger.Printf("Inserted generic attributes for key=%v: attributes=%v\n", key, v)
	}
	return err
}

// Get the attributes for the given key.
// If ok is false, the key is not present and attributes is nil
func (h *GenericAttributeHandler) Get(key string) *GenericAttributes {
	h.mu.RLock()
	defer h.mu.RUnlock()
	attributes, ok := h.Attributes[key]
	if LogLevel >= 5 {
		if ok {
			h.Logger.Printf("Retrieved generic attributes for key=%v: attributes=%v\n", key, attributes)
		} else {
			h.Logger.Printf("Failed to retrieve generic attributes for key=%v: key not found\n", key)
		}
	}
	return &GenericAttributes{Key: key, Attributes: attributes, Ok: ok}
}

// Deletes the attributes for the given key.
// If there is no such element, Delete is a no-op.
func (h *GenericAttributeHandler) Delete(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.Attributes, key)
	if LogLevel >= 5 {
		h.Logger.Printf("Deleted generic attributes for key=%v\n", key)
	}
}

// Starts a goroutine in the background that will
// read keys from the input channel cIn, get their attributes and write them
// to the output channel cOut wrapped in a [GenericAttributes]
func (h *GenericAttributeHandler) Start(cIn chan string) chan *GenericAttributes {
	cOut := make(chan *GenericAttributes, 1)
	go h.getRoutine(cIn, cOut)
	return cOut
}

// Stop any running handlers
func (h *GenericAttributeHandler) Close() {
	h.closed <- true
	close(h.closed)
	if LogLevel >= 1 {
		h.Logger.Println("Generic attribute handler: closed")
	}
}

// Reads keys from cIn, gets their attributes and writes them to cOut
func (h *GenericAttributeHandler) getRoutine(cIn chan string, cOut chan *GenericAttributes) {
	if LogLevel >= 1 {
		h.Logger.Println("Generic attribute handler: started routine")
	}
	for {
		select {
		case key, ok := <-cIn:
			if !ok {
				if LogLevel >= 1 {
					h.Logger.Println("Generic attribute handler: cIn closed, closed cOut and returning")
				}
				close(cOut)
				return
			}
			h.mu.RLock()
			cOut <- h.Get(key)
			h.mu.RUnlock()
		case <-h.closed:
			if LogLevel >= 1 {
				h.Logger.Println("Generic attribute handler: routine received close(), returning")
			}
			close(cOut)
			return
		}
	}
}
