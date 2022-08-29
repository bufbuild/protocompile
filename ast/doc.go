// Copyright 2020-2022 Buf Technologies, Inc.
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

// Package ast defines types for modeling the AST (Abstract Syntax
// Tree) for the protocol buffers source language.
//
// All nodes of the tree implement the Node interface. Leaf nodes in the
// tree implement TerminalNode and all others implement CompositeNode.
// The root of the tree for a proto source file is a *FileNode.
//
// Position information is tracked using a *FileInfo, callings its various
// Add* methods as the file is tokenized by the lexer. This allows AST
// nodes to have a compact representation. To extract detailed position
// information, you must use the nodeInfo method, available on either the
// *FileInfo which produced the node's tokens or the *FileNode root of
// the tree that contains the node.
//
// Comments are not represented as nodes in the tree. Instead, they are
// attributed to terminal nodes in the tree. So, when lexing, comments
// are accumulated until the next non-comment token is found. The AST
// model in this package thus provides access to all comments in the
// file, regardless of location (unlike the SourceCodeInfo present in
// descriptor protos, which is lossy). The comments associated with a
// a non-leaf/non-token node (i.e. a CompositeNode) come from the first
// and last nodes in its sub-tree, for leading and trailing comments
// respectively.
//
// Creation of AST nodes should use the factory functions in this
// package instead of struct literals. Some factory functions accept
// optional arguments, which means the arguments can be nil. If nil
// values are provided for other (non-optional) arguments, the resulting
// node may be invalid and cause panics later in the program.
//
// This package defines numerous interfaces. However, user code should
// not attempt to implement any of them. Most consumers of an AST will
// not work correctly if they encounter concrete implementations other
// than the ones defined in this package.
package ast
