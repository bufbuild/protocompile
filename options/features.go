// Copyright 2020-2023 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package options

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/internal"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/reporter"
)

const (
	// featuresFieldName is the name of a field in every options message.
	featuresFieldName = "features"
	// rawFeaturesFieldName is the name of a field in FeatureSet.
	rawFeaturesFieldName = "raw_features"
)

var (
	featureSetType = (*descriptorpb.FeatureSet)(nil).ProtoReflect().Type()
	featureSetName = featureSetType.Descriptor().FullName()

	errMissingDefault = errors.New("edition is missing default value")
)

// FeaturesResolverCache is a cache of features resolvers. An
// individual resolver is identified by a descriptor for the
// google.protobuf.FeatureSet message. So the cache supports
// multiple versions.
type FeaturesResolverCache struct {
	mu      sync.RWMutex
	entries map[protoreflect.MessageType]*FeaturesResolver
}

// GetResolver returns a FeaturesResolver associated with the given
// descriptor. The given file descriptor must be for the message
// google.protobuf.FeatureSet. An error is returned if the given
// descriptor is not a valid FeatureSet definition.
func (c *FeaturesResolverCache) GetResolver(msgType protoreflect.MessageType) (*FeaturesResolver, error) {
	if c == nil {
		// If nil, no caching. Just go ahead and create a new one.
		if err := validateFeatures(msgType.Descriptor()); err != nil {
			return nil, err
		}
		return &FeaturesResolver{msgType: msgType, defaults: map[string]*featuresDefault{}}, nil
	}

	c.mu.RLock()
	res := c.entries[msgType]
	c.mu.RUnlock()
	if res != nil {
		return res, nil
	}

	if err := validateFeatures(msgType.Descriptor()); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// double-check in case it was concurrently added
	if res := c.entries[msgType]; res != nil {
		return res, nil
	}
	res = &FeaturesResolver{msgType: msgType, defaults: map[string]*featuresDefault{}}
	if c.entries == nil {
		c.entries = map[protoreflect.MessageType]*FeaturesResolver{}
	}
	c.entries[msgType] = res
	return res, nil
}

// FeaturesResolver helps to resolve feature values in source
// files that use editions.
type FeaturesResolver struct {
	msgType  protoreflect.MessageType
	mu       sync.RWMutex
	defaults map[string]*featuresDefault
}

func (r *FeaturesResolver) GetDefaults(edition string) (protoreflect.Message, error) {
	r.mu.RLock()
	res := r.defaults[edition]
	r.mu.RUnlock()
	if res != nil {
		res.maybeInit()
		return res.features, res.err
	}

	res = &featuresDefault{msgType: r.msgType, edition: edition}
	r.mu.Lock()
	if existing := r.defaults[edition]; existing != nil {
		res = existing
	} else {
		r.defaults[edition] = res
	}
	r.mu.Unlock()

	res.maybeInit()
	return res.features, res.err
}

type featuresDefault struct {
	msgType protoreflect.MessageType
	edition string
	init    sync.Once

	features protoreflect.Message
	err      error
}

func (d *featuresDefault) maybeInit() {
	d.init.Do(func() {
		d.features, d.err = populateFeatureDefaults(d.msgType, d.edition)
	})
}

func validateFeatures(md protoreflect.MessageDescriptor) error {
	if md.FullName() != featureSetName {
		return reporter.Errorf(posOf(md), "message name should be %q but instead is %q", featureSetName, md.FullName())
	}
	if md.Oneofs().Len() > 0 {
		ood := md.Oneofs().Get(0)
		return reporter.Errorf(posOf(ood), "FeatureSet has oneof (%s): editions does not support oneof features", ood.Name())
	}
	fields := md.Fields()
	for i, length := 0, fields.Len(); i < length; i++ {
		fld := fields.Get(i)
		switch fld.Cardinality() {
		case protoreflect.Required:
			return reporter.Errorf(posOf(fld), "FeatureSet has required field (%s): editions does not support required features", fld.Name())
		case protoreflect.Repeated:
			return reporter.Errorf(posOf(fld), "FeatureSet has repeated field (%s): editions does not support repeated or map features", fld.Name())
		}
		opts, ok := fld.Options().(*descriptorpb.FieldOptions)
		if !ok {
			return reporter.Errorf(posOf(fld), "FeatureSet field %s: options have incorrect type (%T)", fld.Name(), fld.Options())
		}
		if len(opts.Targets) == 0 {
			return reporter.Errorf(posOf(fld), "FeatureSet field %s is missing target types", fld.Name())
		}
	}
	return nil
}

func posOf(desc protoreflect.Descriptor) ast.SourcePos {
	fd := desc.ParentFile()
	// First see if we can get the source location from the AST
	if result, ok := fd.(linker.Result); ok {
		if msg := findProto(result.FileDescriptorProto(), desc); msg != nil {
			node := result.Node(msg)
			if node != nil {
				return result.FileNode().NodeInfo(node).Start()
			}
		}
		srcLocs := result.FileDescriptorProto().GetSourceCodeInfo().GetLocation()
		if fd.SourceLocations().Len() == 0 && len(srcLocs) > 0 {
			// We haven't yet build the protoreflect.SourceLocations index for this
			// result, so we have to trawl through the proto.
			path, ok := internal.ComputePath(desc)
			if ok {
				for _, srcLoc := range srcLocs {
					if pathsEqual(srcLoc.Path, path) {
						return ast.SourcePos{
							Filename: fd.Path(),
							Line:     int(srcLoc.Span[0] + 1),
							Col:      int(srcLoc.Span[1] + 1),
						}
					}
				}
			}
			return ast.UnknownPos(fd.Path())
		}
	}
	// If not, try to get it from the
	srcLoc := fd.SourceLocations().ByDescriptor(desc)
	if internal.IsZeroLocation(srcLoc) {
		return ast.UnknownPos(fd.Path())
	}
	return ast.SourcePos{
		Filename: fd.Path(),
		Line:     srcLoc.StartLine + 1,
		Col:      srcLoc.StartColumn + 1,
	}
}

func pathsEqual(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findProto(fd *descriptorpb.FileDescriptorProto, element protoreflect.Descriptor) proto.Message {
	if element == nil {
		return nil
	}
	if _, ok := element.(protoreflect.FileDescriptor); ok {
		return fd
	}
	parent := findProto(fd, element.Parent())
	if parent == nil {
		return nil
	}
	switch e := element.(type) {
	case protoreflect.MessageDescriptor:
		switch p := parent.(type) {
		case *descriptorpb.FileDescriptorProto:
			return p.MessageType[e.Index()]
		case *descriptorpb.DescriptorProto:
			return p.NestedType[e.Index()]
		}
	case protoreflect.FieldDescriptor:
		switch p := parent.(type) {
		case *descriptorpb.FileDescriptorProto:
			if e.IsExtension() {
				return p.Extension[e.Index()]
			}
		case *descriptorpb.DescriptorProto:
			if e.IsExtension() {
				return p.Extension[e.Index()]
			}
			return p.Field[e.Index()]
		}
	case protoreflect.OneofDescriptor:
		if p, ok := parent.(*descriptorpb.DescriptorProto); ok {
			return p.OneofDecl[e.Index()]
		}
	case protoreflect.EnumDescriptor:
		switch p := parent.(type) {
		case *descriptorpb.FileDescriptorProto:
			return p.EnumType[e.Index()]
		case *descriptorpb.DescriptorProto:
			return p.EnumType[e.Index()]
		}
	case protoreflect.EnumValueDescriptor:
		if p, ok := parent.(*descriptorpb.EnumDescriptorProto); ok {
			return p.Value[e.Index()]
		}
	case protoreflect.ServiceDescriptor:
		if p, ok := parent.(*descriptorpb.FileDescriptorProto); ok {
			return p.Service[e.Index()]
		}
	case protoreflect.MethodDescriptor:
		if p, ok := parent.(*descriptorpb.ServiceDescriptorProto); ok {
			return p.Method[e.Index()]
		}
	}
	// Shouldn't get here...
	return nil
}

func populateFeatureDefaults(msgType protoreflect.MessageType, edition string) (protoreflect.Message, error) {
	defaults := msgType.New()
	// TODO: how to handle extensions?
	var missing []string
	fields := msgType.Descriptor().Fields()
	for i, length := 0, fields.Len(); i < length; i++ {
		fld := fields.Get(i)
		if fld.Name() == rawFeaturesFieldName {
			// set to empty message
			defaults.Set(fld, defaults.NewField(fld))
			continue
		}
		err := setEditionDefault(defaults, fld, edition)
		if err == errMissingDefault {
			missing = append(missing, string(fld.Name()))
			continue
		} else if err != nil {
			return nil, fmt.Errorf("edition %q is not correctly configured: failed to compute default for feature %s: %w", edition, fld.Name(), err)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("edition %q is not configured: could not compute defaults for features [%s]", edition, strings.Join(missing, ","))
	}
	return defaults, nil
}

func setEditionDefault(msg protoreflect.Message, fld protoreflect.FieldDescriptor, edition string) error {
	opts, ok := fld.Options().(*descriptorpb.FieldOptions)
	if !ok {
		return errMissingDefault
	}
	// Editions inherit defaults from prior editions, so the defaults can be sparse.
	// So we have to search for the greatest edition that is less than or equal to
	// the given one, and use the default value associated with that.
	var matched bool
	var maxMatch string
	var value *string
	// TODO: We do dumb linear scan since defaults are not guaranteed to be
	//   sorted. A possible optimization would be to sort the slice once, when
	//   the FeaturesResolver is initialized, and then binary search.
	for _, def := range opts.EditionDefaults {
		defEdition := def.GetEdition()
		if defEdition == edition || editionLess(defEdition, edition) {
			if !matched || editionLess(maxMatch, defEdition) {
				maxMatch = defEdition
				matched = true
				value = def.Value
			}
		}
	}
	if !matched || value == nil {
		return errMissingDefault
	}
	valStr := *value
	// We use a typed nil so that it won't fall back to the global registry. Features
	// should not use extensions or google.protobuf.Any, so a nil *Types is fine.
	unmarshaler := prototext.UnmarshalOptions{Resolver: (*protoregistry.Types)(nil)}
	// The string value is in the text format: either a field value literal or a
	// message literal. (Repeated and map features aren't supported, so there's no
	// array or map literal syntax to worry about.)
	if fld.Kind() == protoreflect.MessageKind || fld.Kind() == protoreflect.GroupKind {
		fldVal := msg.NewField(fld)
		err := unmarshaler.Unmarshal([]byte(valStr), fldVal.Message().Interface())
		if err != nil {
			return err
		}
		msg.Set(fld, fldVal)
		return nil
	}
	// The value is the textformat for the field. But prototext doesn't provide a way
	// to unmarshal a single field value. To work around, we unmarshal into the enclosing
	// message, so we prefix the value with the field name.
	valStr = fmt.Sprintf("%s: %s", fld.Name(), valStr)
	// Sadly, prototext doesn't support a Merge option. So we can't merge the value
	// directly into the supplied msg. We have to instead unmarshal into an empty
	// message and then use that to set the field in the supplied msg. :(
	empty := msg.Type().New()
	err := unmarshaler.Unmarshal([]byte(valStr), empty.Interface())
	if err != nil {
		return err
	}
	msg.Set(fld, empty.Get(fld))
	return nil
}

// editionLess returns true if a is less than b. This custom sort order was
// transliterated from the C++ implementation in protoc:
//
//	https://github.com/protocolbuffers/protobuf/blob/v24.0-rc2/src/google/protobuf/feature_resolver.cc#L78
func editionLess(a, b string) bool {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if len(aParts[i]) != len(bParts[i]) {
			return len(aParts[i]) < len(bParts[i])
		}
		if aParts[i] != bParts[i] {
			return aParts[i] < bParts[i]
		}
	}
	// Both strings are equal up until an extra element, which makes that string
	// more recent.
	return len(aParts) < len(bParts)
}

func resolveFeatures(features, defaults protoreflect.Message, res linker.Resolver) (protoreflect.Message, error) {
	resolved := proto.Clone(defaults.Interface())
	if features.Descriptor() == defaults.Descriptor() {
		// Same descriptor means we can do this the easy way.
		// Merge the specified features on top of the defaults.
		proto.Merge(resolved, features.Interface())
		// And then try to set the raw_features field to the original
		rawField := defaults.Descriptor().Fields().ByName(rawFeaturesFieldName)
		if rawField != nil && rawField.Message() == features.Descriptor() {
			if features.IsValid() {
				resolved.ProtoReflect().Set(rawField, protoreflect.ValueOfMessage(features))
			} else {
				// features is invalid if it is a nil pointer, which means unset,
				// so also clear the raw field to follow suit
				resolved.ProtoReflect().Clear(rawField)
			}
		}
		return resolved.ProtoReflect(), nil
	}
	// Descriptors don't match, which means we need to merge values by
	// serializing to bytes and then de-serializing.
	data, err := proto.Marshal(features.Interface())
	if err != nil {
		return nil, err
	}
	rawFeatures := defaults.New()
	if err := (proto.UnmarshalOptions{Resolver: res}).Unmarshal(data, rawFeatures.Interface()); err != nil {
		return nil, err
	}
	proto.Merge(resolved, rawFeatures.Interface())
	rawField := defaults.Descriptor().Fields().ByName(rawFeaturesFieldName)
	if rawField != nil && rawField.Message() == defaults.Descriptor() {
		resolved.ProtoReflect().Set(rawField, protoreflect.ValueOfMessage(rawFeatures))
	}
	return resolved.ProtoReflect(), nil
}

func setFeatures(options, features protoreflect.Message, res linker.Resolver) {
	// TODO: signature should return an error; need to handle error in all callers
	fld := options.Descriptor().Fields().ByName(featuresFieldName)
	if fld == nil {
		// no features to resolve
		return
	}
	if fld.IsList() || fld.Message() == nil || fld.Message().FullName() != featureSetName {
		// features field doesn't look right... abort
		// TODO: should this return an error?
		return
	}
	if fld.Message() == features.Descriptor() {
		options.Set(fld, protoreflect.ValueOfMessage(features))
		return
	}
	// Descriptors don't match, which means we need to set the value by
	// serializing to bytes and then de-serializing.
	dest := options.NewField(fld)
	options.Set(fld, dest)
	data, err := proto.Marshal(features.Interface())
	if err != nil {
		return
	}
	if err := (proto.UnmarshalOptions{Resolver: res}).Unmarshal(data, dest.Message().Interface()); err != nil {
		return
	}
}
