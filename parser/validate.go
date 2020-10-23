package parser

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/internal"
	"github.com/jhump/protocompile/reporter"
	"github.com/jhump/protocompile/walk"
)

func validateBasic(res *result, handler *reporter.Handler) {
	fd := res.proto
	isProto3 := fd.GetSyntax() == "proto3"

	_ = walk.DescriptorProtos(fd, func(name protoreflect.FullName, d proto.Message) error {
		switch d := d.(type) {
		case *descriptorpb.DescriptorProto:
			if err := validateMessage(res, isProto3, name, d, handler); err != nil {
				return err
			}
		case *descriptorpb.EnumDescriptorProto:
			if err := validateEnum(res, isProto3, name, d, handler); err != nil {
				return err
			}
		case *descriptorpb.FieldDescriptorProto:
			if err := validateField(res, isProto3, name, d, handler); err != nil {
				return err
			}
		}
		return nil
	})
}

func validateMessage(res *result, isProto3 bool, name protoreflect.FullName, md *descriptorpb.DescriptorProto, handler *reporter.Handler) error {
	scope := fmt.Sprintf("message %s", name)

	if isProto3 && len(md.ExtensionRange) > 0 {
		n := res.ExtensionRangeNode(md.ExtensionRange[0])
		if err := handler.HandleErrorf(n.Start(), "%s: extension ranges are not allowed in proto3", scope); err != nil {
			return err
		}
	}

	if index, err := internal.FindOption(res, handler, scope, md.Options.GetUninterpretedOption(), "map_entry"); err != nil {
		return err
	} else if index >= 0 {
		opt := md.Options.UninterpretedOption[index]
		optn := res.OptionNode(opt)
		md.Options.UninterpretedOption = internal.RemoveOption(md.Options.UninterpretedOption, index)
		valid := false
		if opt.IdentifierValue != nil {
			if opt.GetIdentifierValue() == "true" {
				valid = true
				if err := handler.HandleErrorf(optn.GetValue().Start(), "%s: map_entry option should not be set explicitly; use map type instead", scope); err != nil {
					return err
				}
			} else if opt.GetIdentifierValue() == "false" {
				valid = true
				md.Options.MapEntry = proto.Bool(false)
			}
		}
		if !valid {
			if err := handler.HandleErrorf(optn.GetValue().Start(), "%s: expecting bool value for map_entry option", scope); err != nil {
				return err
			}
		}
	}

	// reserved ranges should not overlap
	rsvd := make(tagRanges, len(md.ReservedRange))
	for i, r := range md.ReservedRange {
		n := res.MessageReservedRangeNode(r)
		rsvd[i] = tagRange{start: r.GetStart(), end: r.GetEnd(), node: n}

	}
	sort.Sort(rsvd)
	for i := 1; i < len(rsvd); i++ {
		if rsvd[i].start < rsvd[i-1].end {
			if err := handler.HandleErrorf(rsvd[i].node.Start(), "%s: reserved ranges overlap: %d to %d and %d to %d", scope, rsvd[i-1].start, rsvd[i-1].end-1, rsvd[i].start, rsvd[i].end-1); err != nil {
				return err
			}
		}
	}

	// extensions ranges should not overlap
	exts := make(tagRanges, len(md.ExtensionRange))
	for i, r := range md.ExtensionRange {
		n := res.ExtensionRangeNode(r)
		exts[i] = tagRange{start: r.GetStart(), end: r.GetEnd(), node: n}
	}
	sort.Sort(exts)
	for i := 1; i < len(exts); i++ {
		if exts[i].start < exts[i-1].end {
			if err := handler.HandleErrorf(exts[i].node.Start(), "%s: extension ranges overlap: %d to %d and %d to %d", scope, exts[i-1].start, exts[i-1].end-1, exts[i].start, exts[i].end-1); err != nil {
				return err
			}
		}
	}

	// see if any extension range overlaps any reserved range
	var i, j int // i indexes rsvd; j indexes exts
	for i < len(rsvd) && j < len(exts) {
		if rsvd[i].start >= exts[j].start && rsvd[i].start < exts[j].end ||
			exts[j].start >= rsvd[i].start && exts[j].start < rsvd[i].end {

			var pos ast.SourcePos
			if rsvd[i].start >= exts[j].start && rsvd[i].start < exts[j].end {
				pos = rsvd[i].node.Start()
			} else {
				pos = exts[j].node.Start()
			}
			// ranges overlap
			if err := handler.HandleErrorf(pos, "%s: extension range %d to %d overlaps reserved range %d to %d", scope, exts[j].start, exts[j].end-1, rsvd[i].start, rsvd[i].end-1); err != nil {
				return err
			}
		}
		if rsvd[i].start < exts[j].start {
			i++
		} else {
			j++
		}
	}

	// now, check that fields don't re-use tags and don't try to use extension
	// or reserved ranges or reserved names
	rsvdNames := map[string]struct{}{}
	for _, n := range md.ReservedName {
		rsvdNames[n] = struct{}{}
	}
	fieldTags := map[int32]string{}
	for _, fld := range md.Field {
		fn := res.FieldNode(fld)
		if _, ok := rsvdNames[fld.GetName()]; ok {
			if err := handler.HandleErrorf(fn.FieldName().Start(), "%s: field %s is using a reserved name", scope, fld.GetName()); err != nil {
				return err
			}
		}
		if existing := fieldTags[fld.GetNumber()]; existing != "" {
			if err := handler.HandleErrorf(fn.FieldTag().Start(), "%s: fields %s and %s both have the same tag %d", scope, existing, fld.GetName(), fld.GetNumber()); err != nil {
				return err
			}
		}
		fieldTags[fld.GetNumber()] = fld.GetName()
		// check reserved ranges
		r := sort.Search(len(rsvd), func(index int) bool { return rsvd[index].end > fld.GetNumber() })
		if r < len(rsvd) && rsvd[r].start <= fld.GetNumber() {
			if err := handler.HandleErrorf(fn.FieldTag().Start(), "%s: field %s is using tag %d which is in reserved range %d to %d", scope, fld.GetName(), fld.GetNumber(), rsvd[r].start, rsvd[r].end-1); err != nil {
				return err
			}
		}
		// and check extension ranges
		e := sort.Search(len(exts), func(index int) bool { return exts[index].end > fld.GetNumber() })
		if e < len(exts) && exts[e].start <= fld.GetNumber() {
			if err := handler.HandleErrorf(fn.FieldTag().Start(), "%s: field %s is using tag %d which is in extension range %d to %d", scope, fld.GetName(), fld.GetNumber(), exts[e].start, exts[e].end-1); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateEnum(res *result, isProto3 bool, name protoreflect.FullName, ed *descriptorpb.EnumDescriptorProto, handler *reporter.Handler) error {
	scope := fmt.Sprintf("enum %s", name)

	if len(ed.Value) == 0 {
		enNode := res.EnumNode(ed)
		if err := handler.HandleErrorf(enNode.Start(), "%s: enums must define at least one value", scope); err != nil {
			return err
		}
	}

	allowAlias := false
	if index, err := internal.FindOption(res, handler, scope, ed.Options.GetUninterpretedOption(), "allow_alias"); err != nil {
		return err
	} else if index >= 0 {
		opt := ed.Options.UninterpretedOption[index]
		valid := false
		if opt.IdentifierValue != nil {
			if opt.GetIdentifierValue() == "true" {
				allowAlias = true
				valid = true
			} else if opt.GetIdentifierValue() == "false" {
				valid = true
			}
		}
		if !valid {
			optNode := res.OptionNode(opt)
			if err := handler.HandleErrorf(optNode.GetValue().Start(), "%s: expecting bool value for allow_alias option", scope); err != nil {
				return err
			}
		}
	}

	if isProto3 && len(ed.Value) > 0 && ed.Value[0].GetNumber() != 0 {
		evNode := res.EnumValueNode(ed.Value[0])
		if err := handler.HandleErrorf(evNode.GetNumber().Start(), "%s: proto3 requires that first value in enum have numeric value of 0", scope); err != nil {
			return err
		}
	}

	if !allowAlias {
		// make sure all value numbers are distinct
		vals := map[int32]string{}
		for _, evd := range ed.Value {
			if existing := vals[evd.GetNumber()]; existing != "" {
				evNode := res.EnumValueNode(evd)
				if err := handler.HandleErrorf(evNode.GetNumber().Start(), "%s: values %s and %s both have the same numeric value %d; use allow_alias option if intentional", scope, existing, evd.GetName(), evd.GetNumber()); err != nil {
					return err
				}
			}
			vals[evd.GetNumber()] = evd.GetName()
		}
	}

	// reserved ranges should not overlap
	rsvd := make(tagRanges, len(ed.ReservedRange))
	for i, r := range ed.ReservedRange {
		n := res.EnumReservedRangeNode(r)
		rsvd[i] = tagRange{start: r.GetStart(), end: r.GetEnd(), node: n}
	}
	sort.Sort(rsvd)
	for i := 1; i < len(rsvd); i++ {
		if rsvd[i].start <= rsvd[i-1].end {
			if err := handler.HandleErrorf(rsvd[i].node.Start(), "%s: reserved ranges overlap: %d to %d and %d to %d", scope, rsvd[i-1].start, rsvd[i-1].end, rsvd[i].start, rsvd[i].end); err != nil {
				return err
			}
		}
	}

	// now, check that fields don't re-use tags and don't try to use extension
	// or reserved ranges or reserved names
	rsvdNames := map[string]struct{}{}
	for _, n := range ed.ReservedName {
		rsvdNames[n] = struct{}{}
	}
	for _, ev := range ed.Value {
		evn := res.EnumValueNode(ev)
		if _, ok := rsvdNames[ev.GetName()]; ok {
			if err := handler.HandleErrorf(evn.GetName().Start(), "%s: value %s is using a reserved name", scope, ev.GetName()); err != nil {
				return err
			}
		}
		// check reserved ranges
		r := sort.Search(len(rsvd), func(index int) bool { return rsvd[index].end >= ev.GetNumber() })
		if r < len(rsvd) && rsvd[r].start <= ev.GetNumber() {
			if err := handler.HandleErrorf(evn.GetNumber().Start(), "%s: value %s is using number %d which is in reserved range %d to %d", scope, ev.GetName(), ev.GetNumber(), rsvd[r].start, rsvd[r].end); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateField(res *result, isProto3 bool, name protoreflect.FullName, fld *descriptorpb.FieldDescriptorProto, handler *reporter.Handler) error {
	scope := fmt.Sprintf("field %s", name)

	node := res.FieldNode(fld)
	if isProto3 {
		if fld.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP {
			if err := handler.HandleErrorf(node.GetGroupKeyword().Start(), "%s: groups are not allowed in proto3", scope); err != nil {
				return err
			}
		} else if fld.Label != nil && fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED {
			if err := handler.HandleErrorf(node.FieldLabel().Start(), "%s: label 'required' is not allowed in proto3", scope); err != nil {
				return err
			}
		} else if fld.Extendee != nil && fld.Label != nil && fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL {
			if err := handler.HandleErrorf(node.FieldLabel().Start(), "%s: label 'optional' is not allowed on extensions in proto3", scope); err != nil {
				return err
			}
		}
		if index, err := internal.FindOption(res, handler, scope, fld.Options.GetUninterpretedOption(), "default"); err != nil {
			return err
		} else if index >= 0 {
			optNode := res.OptionNode(fld.Options.GetUninterpretedOption()[index])
			if err := handler.HandleErrorf(optNode.GetName().Start(), "%s: default values are not allowed in proto3", scope); err != nil {
				return err
			}
		}
	} else {
		if fld.Label == nil && fld.OneofIndex == nil {
			if err := handler.HandleErrorf(node.FieldName().Start(), "%s: field has no label; proto2 requires explicit 'optional' label", scope); err != nil {
				return err
			}
		}
		if fld.GetExtendee() != "" && fld.Label != nil && fld.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REQUIRED {
			if err := handler.HandleErrorf(node.FieldLabel().Start(), "%s: extension fields cannot be 'required'", scope); err != nil {
				return err
			}
		}
	}

	// finally, set any missing label to optional
	if fld.Label == nil {
		fld.Label = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	}

	return nil
}

type tagRange struct {
	start int32
	end   int32
	node  ast.RangeDeclNode
}

type tagRanges []tagRange

func (r tagRanges) Len() int {
	return len(r)
}

func (r tagRanges) Less(i, j int) bool {
	return r[i].start < r[j].start ||
		(r[i].start == r[j].start && r[i].end < r[j].end)
}

func (r tagRanges) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
