package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func FuncCalls(root proto.Message) []*pg_query.FuncCall {
	calls := []*pg_query.FuncCall{}

	var visit func(message protoreflect.Message)

	visit = func(message protoreflect.Message) {
		if call, ok := message.Interface().(*pg_query.FuncCall); ok {
			calls = append(calls, call)
		}

		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			switch {
			case field.IsList() && field.Kind() == protoreflect.MessageKind:
				list := value.List()
				for i := range list.Len() {
					visit(list.Get(i).Message())
				}
			case field.Kind() == protoreflect.MessageKind && !field.IsMap():
				visit(value.Message())
			}

			return true
		})
	}

	if root != nil {
		visit(root.ProtoReflect())
	}

	return calls
}

func FuncName(call *pg_query.FuncCall) string {
	names := StringValues(call.GetFuncname())
	if len(names) == 0 {
		return ""
	}

	return names[len(names)-1]
}
