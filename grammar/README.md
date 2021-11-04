# Introduction
This is a specification for the Protocol Buffer IDL (Interface
Definition Language). Protocol Buffer is also known by the short
hand "protobuf".

The language is a platform-agnostic and implementation-language-agnostic
way of describing data structures and RPC interfaces.

This document is not an official artifact from Google or the Protobuf
team. It has been developed over the course of implementing a [pure-Go
compiler for Protobuf](https://pkg.go.dev/github.com/jhump/protoreflect@v1.10.1/desc/protoparse).
There are official grammars that are available on the Protobuf developer
website ([proto2](https://developers.google.com/protocol-buffers/docs/reference/proto2-spec)
and [proto3](https://developers.google.com/protocol-buffers/docs/reference/proto3-spec)).
However, they are not thorough or entirely accurate and are insufficient
for those interested in developing their own tools that can parse the
language (such as alternate compilers, formatters, linters, or other
static analyzers). This specification attempts to fill that role.

This spec presents a unified grammar, capable of parsing both proto2
and proto3 syntax files. The differences between the two do not
impact the grammar and can be enforced as a post-process over the
resulting parsed syntax tree.

# Notation
The syntax is specified using Extended Backus-Naur Form (EBNF):
```
Production  = production_name "=" Expression "." .
Expression  = Alternative { "|" Alternative } .
Alternative = Term { Term } .
Term        = production_name | token [ "…" token ] | Exclusion | Group | Option | Repetition .
Exclusion   = "!" token | "!" "(" token { "|" token } ")" .
Group       = "(" Expression ")" .
Option      = "[" Expression "]" .
Repetition  = "{" Expression "}" .
```

Productions are expressions constructed from terms and the following operators, in increasing precedence:

* **|**:  Alternation
* **!**:  Exclusion
* **()**: Grouping
* **[]**: Option (0 or 1 times)
* **{}**: Repetition (0 to n times)

Lower-case production names are used to identify lexical tokens. Non-terminals are in CamelCase.
Literal source characters are enclosed in double quotes `""` or back quotes ``` `` ```.
In double-quotes, the contents can encode otherwise non-printable characters. The
backslash character (`\`) is used to mark these encoded sequences:

* `"\n"`: The newline character (code point 10).
* `"\r"`: The carriage return character (code point 13).
* `"\t"`: The horizontal tab character (code point 9).
* `"\v"`: The vertical tab character (code point 11).
* `"\f"`: The form feed character (code point 12).
* `"\xHH"`: Where each H is a hexadecimal character (0 to 9, A to Z). The hexadecimal-encoded
            8-bit value indicates a code point between 0 and 255.
* `"\\"`: A literal backslash character.

These escaped characters represent _bytes_, not Unicode code points (thus the
8-bit limit). To represent literal Unicode code points above 127, a sequence of
bytes representing the UTF-8 encoding of the code point will be used.

A string of multiple characters indicates all characters in a sequence. In other
words, the following two productions are equivalent:
```
foo = "bar"
foo = "b" "a" "r"
```

The exclusion operator is only for use against literal characters and means that
all characters _except for_ the given ones are accepted. For example `!"a"` means
that any character except lower-case `a` is accepted; `!("a"|"b"|"c")` means that
any character except lower-case `a`, `b`, or `c` is accepted.

The form `a … b` represents the set of characters from a through b as alternatives.

# Source Code Representation
Source code is Unicode text encoded in UTF-8. In general, only comments and string literals
can contain code points outside of the range of 7-bit ASCII.

For compatibility with other tools, a source file may contain a UTF-8-encoded byte order mark
(U+FEFF, encoded as `"\xEF\xBB\xBF"`), but only if it is the first Unicode code point in the
source text.

# Lexical Analysis

Parsing a protobuf source file first undergoes lexical analysis. This is the process of
converting the source file, which is a sequence of UTF-8 characters, into a sequence of
_tokens_. (This process is also known as "tokenization".)

Having a tokenization phase allows us to more simply describe the way inputs are transformed
into grammatical elements and how things like whitespace and comments are handled without
cluttering the main grammar.

Tokenization is "greedy", meaning a token matches the longest possible sequence in the input.
That way input like `"0.0.0"`, `"1to3"`, and `"packageio"` can never be interpreted as token
sequences [`"0.0"`, `".0"`]; [`"1"`, `"to"`, `"3"`]; or [`"package"`, `"io"`]
respectively; they will always be interpreted as single tokens.

## Discarded Input

Whitespace is often necessary to separate adjacent tokens in the language. But aside from
that purpose during tokenization, it is ignored. Extra whitespace is allowed anywhere between
tokens. Block comments can also serve to separate tokens, are also allowed anywhere between
tokens, and are also ignored by the grammar.

Protobuf source allows for two styles of comments:
 1. Line comments: These begin with `//` and continue to the end of the line.
 2. Block comments: These begin with `/*` and continue until the first `*/`
    sequence is encountered. A single block comment can span multiple lines.

So the productions below are used to identify whitespace and comments, but they will be
discarded.

If a parser implementation intends to produce descriptor protos that include source code info
(which has details about the location of lexical elements in the file as well as comments)
then the tokenizer should accumulate comments as it scans for tokens so they can be made
available to that later step.
```
whitespace = " " | "\n" | "\r" | "\t" | "\f" | "\v" .
comment = line_comment | block_comment .

line_comment = "/" "/" { !"\n" } .
block_comment = "/" "*" comment_tail .
comment_tail = "*" comment_tail_star | !"*" comment_tail .
comment_tail_star = "/" | "*" comment_tail_star | !("*" | "/") comment_tail .
```

If the `/*` sequence is found to start a block comment, but the above rule is not
matched, it indicates a malformed block comment: EOF was reached before the
concluding `*/` sequence was found. Such a malformed comment is a syntax
error.

If a comment text contains a null character (code point zero) then it is malformed
and a syntax error should be reported.

## Character Classes

The following categories for input characters are used through the lexical analysis
productions in the following sections:
```
letter        = "A" … "Z" | "a" … "z" | "_" .
decimal_digit = "0" … "9" .
octal_digit   = "0" … "7" .
hex_digit     = "0" … "9" | "A" … "F" | "a" … "f" .

byte_order_mark = "\xEF\xBB\xBF"
```

The `byte_order_mark` byte sequence is the UTF-8 encoding of the byte-order mark
character (U+FEFF).

## Tokens

The result of lexical analysis is a stream of tokens of the following kinds:
 * `identifier`
 * 42 token types corresponding to keywords.
 * `int_literal`
 * `float_literal`
 * `string_literal`
 * 15 token types corresponding to symbols, punctuation, and operators.

### Identifiers

An identifier is used for named elements in the protobuf language, like names
of messages, fields, and services. There are 42 keywords in the protobuf grammar
that may also be used as identifiers.
```
identifier = letter { letter | decimal_digit } .
```

When an `identifier` is found, if it matches a keyword, its token type is changed
to match the keyword, per the rules below. All of the keyword token types below
are *also* considered identifiers by the grammar. For example, a production in the
grammar that references `identifier` will also accept `syntax` or `map`.
```
syntax   = "syntax" .      float    = "float" .       group      = "group" .
import   = "import" .      int32    = "int32" .       oneof      = "oneof" .
weak     = "weak" .        int64    = "int64" .       map        = "map" .
public   = "public" .      uint32   = "uint32" .      extensions = "extensions" .
package  = "package" .     uint64   = "uint64" .      to         = "to" .
option   = "option" .      sint32   = "sint32" .      max        = "max" .
true     = "true" .        sint64   = "sint64" .      reserved   = "reserved" .
false    = "false" .       fixed32  = "fixed32" .     enum       = "enum" .
inf      = "inf" .         fixed64  = "fixed64" .     message    = "message" .
nan      = "nan" .         sfixed32 = "sfixed32" .    extend     = "extend" .
repeated = "repeated" .    sfixed64 = "sfixed64" .    service    = "service" .
optional = "optional" .    bool     = "bool" .        rpc        = "rpc" .
required = "required" .    string   = "string" .      stream     = "stream" .
double   = "double" .      bytes    = "bytes" .       returns    = "returns" .
```

### Numeric Literals

Handling of numeric literals is a bit special in order to avoid a situation where
`"0.0.0"` or `"100to3"` is tokenized as [`"0.0"`, `".0"`] or [`"100"`, `"to"`, `"3"`]
respectively. Instead of these input sequences representing a possible sequence of 2
or more tokens, they are considered invalid numeric literals.

So input is first scanned for the `numeric_literal` token type:
```
numeric_literal = ( "." | decimal_digit ) { digit_or_point }
                  { ( "e" | "E" ) [ "+" | "-" ] digit_or_point { digit_or_point } }

digit_or_point = "." | decimal_digit | letter
```
 
When a `numeric_literal` token is found, it is then checked to see if it matches the `int_literal`
or `float_literal` rules (see below). If it does then the scanned token is included in the
result token stream with `int_literal` or `float_literal` as its token type. But if it does *not*
match, it is a malformed numeric literal which is considered a syntax error.

Below is the rule for `int_literal`:
```
int_literal = decimal_literal | octal_literal | hex_literal .

decimal_literal = "0" | ( "1" … "9" ) [ decimal_digits ] .
octal_literal   = "0" octal_digits .
hex_literal     = "0" ( "x" | "X" ) hex_digits .
decimal_digits  = decimal_digit { decimal_digit } .
octal_digits    = octal_digit { octal_digit } .
hex_digits      = hex_digit { hex_digit } .
```

Below is the rule for `float_literal`:
```
float_literal = decimal_digits "." [ decimal_digits ] [ decimal_exponent ] |
                decimal_digits decimal_exponent |
                "." decimal_digits [ decimal_exponent ] .

decimal_exponent  = ( "e" | "E" ) [ "+" | "-" ] decimal_digits .
```

### String Literals

String values include C-style support for escape sequences. String literals are used
for constant values of `bytes` fields, so they must be able to represent arbitrary
binary data, in addition to normal/valid UTF-8 strings.

Note that protobuf explicitly disallows a null character (code point 0) to appear in
the string, but an _encoded null_ (e.g. `"\x00"`) can appear.
```
string_literal = single_quoted_string_literal | double_quoted_string_literal .

single_quoted_string_literal = "'" { !("\n" | "\x00" | "'" | `\`) | rune_escape_seq } "'" .
double_quoted_string_literal = `"` { !("\n" | "\x00" | `"` | `\`) | rune_escape_seq } `"` .

rune_escape_seq    = simple_escape_seq | hex_escape_seq | octal_escape_seq | unicode_escape_seq .
simple_escape_seq  = `\` ( "a" | "b" | "f" | "n" | "r" | "t" | "v" | `\` | "'" | `"` | "?" ) .
hex_escape_seq     = `\` "x" hex_digit hex_digit .
octal_escape_seq   = `\` octal_digit [ octal_digit [ octal_digit ] ] .
unicode_escape_seq = `\` "u" hex_digit hex_digit hex_digit hex_digit |
                     `\` "U" hex_digit hex_digit hex_digit hex_digit
                             hex_digit hex_digit hex_digit hex_digit .
```

### Punctuation and Operators

The symbols below represent all other valid input characters used by the protobuf grammar.
```
semicolon = ";" .   colon     = ":" .   plus      = "+" .   l_brace   = "{" .   r_bracket = "]" .
comma     = "," .   equals    = "=" .   l_paren   = "(" .   r_brace   = "}" .   l_angle   = "<" .
dot       = "." .   minus     = "-" .   r_paren   = ")" .   l_bracket = "[" .   r_angle   = ">" .
```

# Grammar

The productions below define the grammar rules for the protobuf IDL.

## Files

The `File` production represents the contents of a valid protobuf source file.
```
File = [ byte_order_mark ] [ SyntaxDecl ] { FileElement } .

FileElement = ImportDecl |
              PackageDecl |
              OptionDecl |
              MessageDecl |
              EnumDecl |
              ExtensionDecl |
              ServiceDecl |
              EmptyDecl .
EmptyDecl   = semicolon .
```

### Syntax

Files should define a syntax. The string literal must have a value of
"proto2" or "proto3". Other syntax values are not allowed. If a file
contains no syntax statement then proto2 is the assumed syntax.

String literals support C-style concatenation. So the sequence
`"prot" "o2"` is equivalent to `"proto2"`.
```
SyntaxDecl = syntax equals StringLiteral semicolon .

StringLiteral = string_literal { string_literal } .
```

### Package Declaration

A file can only include a single package declaration, though it
can appear anywhere in the file (except before the syntax).

Packages use dot-separated namespace components. A compound name
like `foo.bar.baz` represents a nesting of namespaces, with `foo`
being the outermost namespace, then `bar`, and finally `baz`.
So all of the elements in two files, with packages `foo.bar.baz`
and`foo.bar.buzz` for example, are in the `foo` and `foo.bar`
namespaces.
```
PackageDecl = package QualifiedIdentifier semicolon .

QualifiedIdentifier = identifier { dot identifier } .
```

### Imports

In order to refer to messages and enum types defined in another
file, that file must be imported.

A "public" import means that everything in that file is treated
as if it were defined in the importing file, for purpose of
transitive importers. For example, if file "a.proto" imports
"b.proto" and "b.proto" has "c.proto" as a _public_ import, then
the elements in "a.proto" may refer to elements defined in
"c.proto", even though "a.proto" does not directly import "c.proto".
```
ImportDecl = import [ weak | public ] StringLiteral semicolon .
```

## Options

Many elements defined in a protobuf source file allow options
which provide a way to customize behavior and also provide the
ability to use custom annotations on elements (which can then be
used by protoc plugins or runtime libraries).
```
OptionDecl = option OptionName equals OptionValue semicolon .

OptionName = ( identifier | l_paren TypeName r_paren ) [ dot OptionName ] .
TypeName   = [ dot ] QualifiedIdentifier .
```

Option values are literals. In addition to primitive literal values
(like integers, floating point numbers, strings, and booleans), option
values can also be aggregate values (message literals). This aggregate
must be enclosed in braces (`{` and `}`).

The syntax for the value _inside_ the braces, however, is the protobuf
text format. This means nested message values therein may be enclosed in
braces or may instead be enclosed in angle brackets (`<` and `>`). In
message values, a single field is defined by a field name and value,
separated by a colon. However, the colon is optional if the value is a
composite (e.g. will be surrounded by braces or brackets).

List literals may not be used directly as option values (even for
repeated fields) but are allowed inside a message value, for a
repeated field.
```
OptionValue = ScalarValue | MessageLiteralWithBraces .

ScalarValue        = StringLiteral | BoolLiteral | NumLiteral | identifier .
BoolLiteral        = true | false .
NumLiteral         = nan | [ minus | plus ] UnsignedNumLiteral .
UnsignedNumLiteral = float_literal | int_literal | inf .

MessageLiteralWithBraces = l_brace { MessageLiteralField } r_brace .
MessageLiteralField      = MessageLiteralFieldName colon Value |
                           MessageLiteralFieldName CompositeValue .
MessageLiteralFieldName  = identifier | l_bracket TypeName r_bracket .
Value                    = ScalarValue | CompositeValue .
CompositeValue           = MessageLiteral | ListLiteral .
MessageLiteral           = MessageLiteralWithBraces |
                           l_angle { MessageLiteralField } r_angle .

ListLiteral = l_bracket [ ListElement { comma ListElement } ] r_bracket .
ListElement = ScalarValue | MessageLiteral .
```

## Messages

The core of the protobuf IDL is defining messages, which are heterogenous
composite data types.

Files whose syntax declaration indicates "proto3" are not allowed to
include `GroupDecl` or `ExtensionRangeDecl` elements.
```
MessageDecl = message identifier l_brace { MessageElement } r_brace .

MessageElement = FieldDecl |
                 MapFieldDecl |
                 GroupDecl |
                 OneofDecl |
                 OptionDecl |
                 ExtensionRangeDecl |
                 MessageReservedDecl |
                 EnumDecl |
                 MessageDecl |
                 ExtensionDecl |
                 EmptyDecl .
```

### Fields

Field declarations are found inside messages. They can also be found inside
`extends` blocks, for defining extension fields. Each field indicates its
cardinality (`required`, `optional`, or `repeated`; also called the field's label),
its type, its name, its tag number, and (optionally) options.

Field declarations in the proto2 syntax *require* a label token.

Declarations in proto3 are not allowed to use `required` labels and may omit
the `optional` label. When the label is omitted, the subsequent type name
may *not* start with an identifier whose text could be ambiguous with other
kinds of elements in this scope. So such field declarations in a message declaration
may not have a type name that starts with any of the following identifiers:
   * "message"
   * "enum"
   * "oneof"
   * "extensions"
   * "reserved"
   * "extend"
   * "option"
   * "optional"
   * "required"
   * "repeated"

Similarly, a field declaration in an `extends` block may not have a type name
that starts with any of the following identifiers:
   * "optional"
   * "required"
   * "repeated"

Note that it is acceptable if the above words are _prefixes_ of the first token in
the type name. For example, inside a message a type name "enumeration" is allowed, even
though it starts with "enum". But a name of "enum.Statuses" would not be allowed, because
the first constituent token is "enum". A _fully_qualified_ type name (one that starts with
a dot) is always accepted, regardless of the first identifier token, since the dot prevents
ambiguity.
```
FieldDecl = [ required | optional | repeated ] TypeName identifier equals int_literal
            [ CompactOptions ] semicolon .

CompactOptions = l_bracket CompactOption { comma CompactOption } r_bracket .
CompactOption  = OptionName equals OptionValue .
```

Map fields never have a label as their cardinality is implicitly repeated (since
a map can have more than one entry).
```
MapFieldDecl = MapType identifier equals int_literal [ CompactOptions ] semicolon .

MapType    = map l_angle MapKeyType comma TypeName r_angle .
MapKeyType = int32   | int64   | uint32   | uint64   | sint32 | sint64 |
             fixed32 | fixed64 | sfixed32 | sfixed64 | bool   | string .
```

Groups are a mechanism in proto2 to create a field that is a nested message.
The message definition is inlined into the group field declaration.

The group's name must start with a capital letter. In some contexts, the group field
goes by the lower-cased form of this name.
```
GroupDecl = [ required | optional | repeated ] group identifier equals int_literal
            [ CompactOptions ] l_brace { MessageElement } r_brace .
```

### Oneofs

A "oneof" is a set of fields that act like a discriminated union.

Files whose syntax declaration indicates "proto3" are not allowed to
include `OneofGroupDecl` elements.
```
OneofDecl = oneof identifier l_brace { OneofElement } r_brace .

OneofElement = OptionDecl |
               OneofFieldDecl |
               OneofGroupDecl |
               EmptyDecl .
```

Fields in a oneof always omit the label (`required`, `optional`, or `repeated`) and
are always optional. They follow the same restrictions as other field declarations
that have no leading label: the first token of the `TypeName` may not be an
`identifier` whose text could be ambiguous with other elements. They also may not
match any of the label keywords. To that end, fields in a oneof may not have a type
name that starts with any of the following:
  * "option"
  * "optional"
  * "required"
  * "repeated"
```
OneofFieldDecl = TypeName identifier equals int_literal
                 [ CompactOptions ] semicolon .
```

A group's name must start with a capital letter. In some contexts, the group field
goes by the lower-cased form of this name.
```
OneofGroupDecl = group identifier equals int_literal
                 [ CompactOptions ] l_brace { MessageElement } r_brace .
```

### Extension Ranges

Extendable messages (proto2 syntax only) may define ranges of tags. Extension fields
must use a tag in one of these ranges.
```
ExtensionRangeDecl = extensions TagRange { comma TagRange } [ CompactOptions ] semicolon .

TagRange = int_literal [ to ( int_literal | max ) ] .
```

### Reserved Names and Numbers

Messages can reserve field names and numbers to prevent them from being used.
This is typically to prevent old tag numbers and names from being recycled.
```
MessageReservedDecl = reserved ( TagRange { comma TagRange } | Names ) semicolon .

Names = String { comma String }
```

## Enums

Enums represent an enumerated type, where values must be one of the defined
enum values.
```
EnumDecl = enum identifier l_brace { EnumElement } r_brace .

EnumElement = OptionDecl |
              EnumValueDecl |
              EnumReservedDecl |
              EmptyDecl .
```

Value names (the first `identifier` token) may not match any of these keywords:
  * "reserved"
  * "option"
```
EnumValueDecl = identifier equals SignedIntLiteral [ CompactOptions ] semicolon .

SignedIntLiteral = [ minus ] int_literal .
```

Like messages, enums can also reserve names and numbers, typically to prevent
recycling names and numbers from old enum values.
```
EnumReservedDecl = reserved ( EnumValueRange { comma EnumValueRange } | Names ) semicolon .

EnumValueRange = SignedIntLiteral [ to ( SignedIntLiteral | max ) ] .
```

## Extensions

Extensions are allowed in both proto2 and proto3, even though an _extendable
message_ can only be defined in a file with proto2 syntax.

However, a file with proto3 syntax is not allowed to use the `GroupDecl` rule
as groups are not supported in proto3.
```
ExtensionDecl = extend TypeName l_brace { ExtensionElement } r_brace .

ExtensionElement = FieldDecl |
                   GroupDecl |
                   EmptyDecl .
```

## Services

Services are used to define RPC interfaces. Each service is a collection
of RPC methods.
```
ServiceDecl = service identifier l_brace { ServiceElement } r_brace .

ServiceElement = OptionDecl |
                 RpcDecl |
                 EmptyDecl .
```

Each RPC defines a single method/operation and its request and response types.
```
RpcDecl = rpc identifier RpcType returns RpcType semicolon |
          rpc identifier RpcType returns RpcType l_brace { RpcElement } r_brace .

RpcType    = l_paren [ stream ] TypeName r_paren .
RpcElement = OptionDecl |
             EmptyDecl .
```
