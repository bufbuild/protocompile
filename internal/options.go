package internal

import (
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/jhump/protocompile/ast"
	"github.com/jhump/protocompile/reporter"
)

type hasOptionNode interface {
	OptionNode(part *descriptorpb.UninterpretedOption) ast.OptionDeclNode
	// This is needed to be able to get NodeInfo
	FileNode() ast.FileDeclNode
}

func FindOption(res hasOptionNode, handler *reporter.Handler, scope string, opts []*descriptorpb.UninterpretedOption, name string) (int, error) {
	found := -1
	for i, opt := range opts {
		if len(opt.Name) != 1 {
			continue
		}
		if opt.Name[0].GetIsExtension() || opt.Name[0].GetNamePart() != name {
			continue
		}
		if found >= 0 {
			optNode := res.OptionNode(opt)
			fn := res.FileNode()
			node := optNode.GetName()
			nodeInfo := fn.NodeInfo(node)
			return -1, handler.HandleErrorf(nodeInfo.Start(), "%s: option %s cannot be defined more than once", scope, name)
		}
		found = i
	}
	return found, nil
}

func RemoveOption(uo []*descriptorpb.UninterpretedOption, indexToRemove int) []*descriptorpb.UninterpretedOption {
	if indexToRemove == 0 {
		return uo[1:]
	} else if indexToRemove == len(uo)-1 {
		return uo[:len(uo)-1]
	} else {
		return append(uo[:indexToRemove], uo[indexToRemove+1:]...)
	}
}
