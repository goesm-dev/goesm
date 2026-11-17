// Command protolegacy loads messages of the legacy protobuf API, whose
// Descriptor method returns their file descriptor compressed with gzip, as
// protoc-gen-go wrote them before 2018. goesm patches the decompression
// (internal/natives/patch/google.golang.org/protobuf/internal/impl), so
// this compresses the descriptor in the ways compress/gzip can (stored,
// Huffman-only, fixed and dynamic codes, header fields, several members)
// and breaks it in the ways it rejects.
package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/descriptorpb"
)

// descriptor returns the FileDescriptorProto of a file with one message,
// legacy.Item, of a string, an int32 and a repeated string field.
func descriptor() []byte {
	field := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{Name: proto.String(name), JsonName: proto.String(name), Number: proto.Int32(num), Type: typ.Enum(), Label: label.Enum()}
	}
	opt, rep := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL, descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("legacy/item.proto"),
		Package: proto.String("legacy"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Item"),
			Field: []*descriptorpb.FieldDescriptorProto{
				field("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt),
				field("count", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32, opt),
				field("tags", 3, descriptorpb.FieldDescriptorProto_TYPE_STRING, rep),
			},
		}},
	}
	// More messages with long names make the descriptor compress with
	// dynamic codes and matches far back.
	for i := range 60 {
		name := fmt.Sprintf("Extra%d%s", i, strings.Repeat("Abcdefgh"[i%8:], i%7+1))
		fd.MessageType = append(fd.MessageType, &descriptorpb.DescriptorProto{
			Name:  proto.String(name),
			Field: []*descriptorpb.FieldDescriptorProto{field(strings.ToLower(name), int32(i+1), descriptorpb.FieldDescriptorProto_TYPE_BYTES, rep)},
		})
	}
	b, err := proto.Marshal(fd)
	if err != nil {
		panic(err)
	}
	return b
}

// compress gzips b at level, with the header h.
func compress(b []byte, level int, h gzip.Header) []byte {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		panic(err)
	}
	w.Header = h
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

var variants []struct {
	name string
	gz   []byte
}

func add(name string, gz []byte) {
	// A copy: descriptors are cached by the address of their first byte.
	variants = append(variants, struct {
		name string
		gz   []byte
	}{name, append([]byte(nil), gz...)})
}

// The legacy message type: a distinct type for each variant, as the
// descriptor of a type is loaded once.
type kind interface{ index() int }

type item[K kind] struct {
	Name  string   `protobuf:"bytes,1,opt,name=name,proto3"`
	Count int32    `protobuf:"varint,2,opt,name=count,proto3"`
	Tags  []string `protobuf:"bytes,3,rep,name=tags,proto3"`
}

func (m *item[K]) Reset()         { *m = item[K]{} }
func (m *item[K]) String() string { return m.Name }
func (*item[K]) ProtoMessage()    {}
func (*item[K]) Descriptor() ([]byte, []int) {
	var k K
	return variants[k.index()].gz, []int{0}
}

type (
	k0  struct{}
	k1  struct{}
	k2  struct{}
	k3  struct{}
	k4  struct{}
	k5  struct{}
	k6  struct{}
	k7  struct{}
	k8  struct{}
	k9  struct{}
	k10 struct{}
	k11 struct{}
	k12 struct{}
	k13 struct{}
	k14 struct{}
)

func (k0) index() int  { return 0 }
func (k1) index() int  { return 1 }
func (k2) index() int  { return 2 }
func (k3) index() int  { return 3 }
func (k4) index() int  { return 4 }
func (k5) index() int  { return 5 }
func (k6) index() int  { return 6 }
func (k7) index() int  { return 7 }
func (k8) index() int  { return 8 }
func (k9) index() int  { return 9 }
func (k10) index() int { return 10 }
func (k11) index() int { return 11 }
func (k12) index() int { return 12 }
func (k13) index() int { return 13 }
func (k14) index() int { return 14 }

// load wraps m as a protoreflect.Message, which loads its descriptor, and
// prints the descriptor and the message's fields, or the panic.
func load(name string, m any) {
	defer func() {
		if r := recover(); r != nil {
			err, _ := r.(error)
			var ce flate.CorruptInputError
			switch {
			case errors.As(err, &ce):
				// The offset is the one in the flate stream under native
				// Go, in the gzip data under goesm.
				fmt.Printf("%s: panic: flate: corrupt input\n", name)
			default:
				fmt.Printf("%s: panic: %v\n", name, r)
			}
		}
	}()
	pm := protoimpl.X.ProtoMessageV2Of(m).ProtoReflect()
	d := pm.Descriptor()
	var fields []string
	for i := 0; i < d.Fields().Len(); i++ {
		f := d.Fields().Get(i)
		fields = append(fields, fmt.Sprintf("%s=%d:%v", f.Name(), f.Number(), f.Kind()))
	}
	var values []string
	pm.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if f.IsList() {
			for i := 0; i < v.List().Len(); i++ {
				values = append(values, fmt.Sprintf("%s:%v", f.Name(), v.List().Get(i)))
			}
		} else {
			values = append(values, fmt.Sprintf("%s:%v", f.Name(), v))
		}
		return true
	})
	file := d.ParentFile()
	last := file.Messages().Get(file.Messages().Len() - 1)
	fmt.Printf("%s: %s %s %v %v %v %d %s %s\n", name, d.FullName(), file.Path(), file.Syntax(), fields, values, file.Messages().Len(), last.FullName(), last.Fields().Get(0).JSONName())
}

func main() {
	raw := descriptor()
	fmt.Println("descriptor:", len(raw), crc32.ChecksumIEEE(raw))
	def := compress(raw, gzip.DefaultCompression, gzip.Header{})
	add("default", def)
	add("stored", compress(raw, gzip.NoCompression, gzip.Header{}))
	add("huffman-only", compress(raw, gzip.HuffmanOnly, gzip.Header{}))
	add("best-speed", compress(raw, gzip.BestSpeed, gzip.Header{}))
	add("header", compress(raw, gzip.BestCompression, gzip.Header{Name: "item.proto", Comment: "legacy é", Extra: []byte("xyz")}))
	// Fixed codes: a short member compresses with the fixed Huffman codes.
	half := len(raw) / 2
	add("members", append(compress(raw[:half], gzip.DefaultCompression, gzip.Header{}), compress(raw[half:], gzip.NoCompression, gzip.Header{})...))
	small := append(compress(raw[:20], gzip.BestCompression, gzip.Header{}), compress(raw[20:], gzip.DefaultCompression, gzip.Header{})...)
	add("fixed", small)
	// A header CRC (FHCRC), which compress/gzip checks but does not write.
	hcrc := append([]byte(nil), def...)
	hcrc[3] |= 0x02
	sum := crc32.ChecksumIEEE(hcrc[:10])
	hcrc = append(hcrc[:10:10], append([]byte{byte(sum), byte(sum >> 8)}, def[10:]...)...)
	add("header-crc", hcrc)

	badHCRC := append([]byte(nil), hcrc...)
	badHCRC[10] ^= 1
	add("bad-header-crc", badHCRC)
	badMagic := append([]byte(nil), def...)
	badMagic[1] = 0
	add("bad-magic", badMagic)
	add("truncated", def[:len(def)-20])
	add("truncated-trailer", def[:len(def)-3])
	badSum := append([]byte(nil), def...)
	badSum[len(badSum)-6] ^= 0xff
	add("bad-checksum", badSum)
	badBlock := append([]byte(nil), def...)
	badBlock[10] |= 0x06 // block type 3
	add("bad-block", badBlock)
	add("trailing-garbage", append(append([]byte(nil), def...), 1, 2, 3))

	msgs := []any{
		&item[k0]{Name: "a", Count: 1, Tags: []string{"x"}}, &item[k1]{Name: "b"}, &item[k2]{Count: 3}, &item[k3]{Tags: []string{"y", "z"}},
		&item[k4]{Name: "e"}, &item[k5]{Name: "f"}, &item[k6]{Name: "g"}, &item[k7]{Name: "h"},
		&item[k8]{}, &item[k9]{}, &item[k10]{}, &item[k11]{}, &item[k12]{}, &item[k13]{}, &item[k14]{},
	}
	for i, v := range variants {
		load(v.name, msgs[i])
	}
}
