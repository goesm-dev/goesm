//go:build goesm

package impl

import (
	"compress/flate"
	"compress/gzip"
	"hash/crc32"
	"io"
)

// legacyLoadFileDesc unmarshals b as a compressed FileDescriptorProto message.
//
// The original decompresses b with compress/gzip, through io.Reader, whose
// Read blocks for the readers that wait on the host, such as an HTTP
// response body read from fetch or an io.Pipe. goesm decides which
// functions are async by the methods an interface call may reach, so
// legacyLoadFileDesc became async, and with it, through the Descriptor
// method of legacy messages, the String method of every message,
// everything that formats with fmt (which calls String) and every caller
// of those, such as protojson's decoder, which formats its syntax errors.
// A Connect client's unary call spent a third of its time in async calls.
//
// This decompresses the slice itself, synchronously, with the inflate
// algorithm of Mark Adler's puff (RFC 1951), and the header and trailer
// checks of compress/gzip (RFC 1952), so a well-formed b gives the same
// bytes. A malformed one panics with the error compress/gzip or
// compress/flate return for it (gzip.ErrHeader, gzip.ErrChecksum,
// io.ErrUnexpectedEOF, flate.CorruptInputError), the offset of a
// flate.CorruptInputError being one in b. The whole decoder is inside this
// function, as a patch of a module can only replace declarations.
func legacyLoadFileDesc(b []byte) protoreflect.FileDescriptor {
	// Fast-path: check whether we already have a cached file descriptor.
	if fd, ok := legacyFileDescCache.Load(&b[0]); ok {
		return fd.(protoreflect.FileDescriptor)
	}

	// Slow-path: decompress and unmarshal the file descriptor proto.
	var (
		pos  int    // the next byte of b
		bits uint32 // bits read from b and not used yet, the next in the low bits
		nbit uint
		out  []byte
		base int // len(out) at the start of the member: distances do not reach before it
	)
	le16 := func(i int) int { return int(b[i]) | int(b[i+1])<<8 }
	le32 := func(i int) uint32 { return uint32(le16(i)) | uint32(le16(i+2))<<16 }
	corrupt := func() error { return flate.CorruptInputError(pos) }
	// need returns the next n (at most 16) bits.
	need := func(n uint) (int, error) {
		for nbit < n {
			if pos >= len(b) {
				return 0, io.ErrUnexpectedEOF
			}
			bits |= uint32(b[pos]) << nbit
			pos++
			nbit += 8
		}
		v := int(bits & (1<<n - 1))
		bits >>= n
		nbit -= n
		return v, nil
	}
	// A canonical Huffman code: the number of codes of each length, and
	// the symbols in the order of their codes.
	type huffman struct {
		count  [16]int
		symbol []int
	}
	// build sets h to the code of lengths. Like compress/flate, it accepts
	// an empty code, which fails when used, and a single code of length 1,
	// and rejects other incomplete codes.
	build := func(h *huffman, lengths []int) bool {
		h.count = [16]int{}
		max := 0
		for _, l := range lengths {
			h.count[l]++
			if l > max {
				max = l
			}
		}
		if max == 0 {
			return true
		}
		left := 1
		for l := 1; l < 16; l++ {
			left = left<<1 - h.count[l]
			if left < 0 {
				return false
			}
		}
		if left != 0 && !(max == 1 && h.count[1] == 1) {
			return false
		}
		var offs [16]int
		for l := 1; l < 15; l++ {
			offs[l+1] = offs[l] + h.count[l]
		}
		h.symbol = make([]int, len(lengths))
		for sym, l := range lengths {
			if l != 0 {
				h.symbol[offs[l]] = sym
				offs[l]++
			}
		}
		return true
	}
	// decode reads a symbol of the code h, a bit at a time.
	decode := func(h *huffman) (int, error) {
		code, first, index := 0, 0, 0
		for l := 1; l < 16; l++ {
			bit, err := need(1)
			if err != nil {
				return 0, err
			}
			code |= bit
			count := h.count[l]
			if code-first < count {
				return h.symbol[index+code-first], nil
			}
			index += count
			first = (first + count) << 1
			code <<= 1
		}
		return 0, corrupt()
	}
	lenBase := [29]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
	lenExtra := [29]uint{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
	dstBase := [30]int{1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577}
	dstExtra := [30]uint{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}
	// codes decodes the symbols of a compressed block up to its end.
	codes := func(lit, dst *huffman) error {
		for {
			sym, err := decode(lit)
			if err != nil {
				return err
			}
			if sym < 256 {
				out = append(out, byte(sym))
				continue
			}
			if sym == 256 {
				return nil
			}
			sym -= 257
			if sym >= 29 {
				return corrupt()
			}
			extra, err := need(lenExtra[sym])
			if err != nil {
				return err
			}
			n := lenBase[sym] + extra
			if sym, err = decode(dst); err != nil {
				return err
			}
			if sym >= 30 {
				return corrupt()
			}
			if extra, err = need(dstExtra[sym]); err != nil {
				return err
			}
			d := dstBase[sym] + extra
			if d > len(out)-base {
				return corrupt()
			}
			for ; n > 0; n-- {
				out = append(out, out[len(out)-d])
			}
		}
	}
	stored := func() error {
		bits, nbit = 0, 0 // the rest of the byte is padding
		if len(b)-pos < 4 {
			return io.ErrUnexpectedEOF
		}
		n := le16(pos)
		if le16(pos+2) != n^0xffff {
			return corrupt()
		}
		pos += 4
		if len(b)-pos < n {
			return io.ErrUnexpectedEOF
		}
		out = append(out, b[pos:pos+n]...)
		pos += n
		return nil
	}
	fixed := func() error {
		var lengths [288 + 30]int
		for i := 0; i < len(lengths); i++ {
			switch {
			case i < 144:
				lengths[i] = 8
			case i < 256:
				lengths[i] = 9
			case i < 280:
				lengths[i] = 7
			case i < 288:
				lengths[i] = 8
			default:
				lengths[i] = 5
			}
		}
		var lit, dst huffman
		build(&lit, lengths[:288])
		build(&dst, lengths[288:])
		return codes(&lit, &dst)
	}
	dynamic := func() error {
		nlen, err := need(5)
		if err != nil {
			return err
		}
		ndst, err := need(5)
		if err != nil {
			return err
		}
		ncode, err := need(4)
		if err != nil {
			return err
		}
		nlen, ndst, ncode = nlen+257, ndst+1, ncode+4
		if nlen > 286 || ndst > 30 {
			return corrupt()
		}
		order := [19]int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}
		var lengths [286 + 30]int
		for i := 0; i < ncode; i++ {
			if lengths[order[i]], err = need(3); err != nil {
				return err
			}
		}
		var cl huffman
		if !build(&cl, lengths[:19]) {
			return corrupt()
		}
		for i := 0; i < nlen+ndst; {
			sym, err := decode(&cl)
			if err != nil {
				return err
			}
			if sym < 16 {
				lengths[i] = sym
				i++
				continue
			}
			l, rep := 0, 0
			switch sym {
			case 16:
				if i == 0 {
					return corrupt()
				}
				l = lengths[i-1]
				rep, err = need(2)
				rep += 3
			case 17:
				rep, err = need(3)
				rep += 3
			default:
				rep, err = need(7)
				rep += 11
			}
			if err != nil {
				return err
			}
			if i+rep > nlen+ndst {
				return corrupt()
			}
			for ; rep > 0; rep-- {
				lengths[i] = l
				i++
			}
		}
		if lengths[256] == 0 {
			return corrupt() // no end-of-block code
		}
		var lit, dst huffman
		if !build(&lit, lengths[:nlen]) || !build(&dst, lengths[nlen:nlen+ndst]) {
			return corrupt()
		}
		return codes(&lit, &dst)
	}
	// member decodes a gzip member at b[pos:], as gzip.Reader does.
	member := func() error {
		start := pos
		if len(b)-pos < 10 {
			return io.ErrUnexpectedEOF
		}
		if b[pos] != 0x1f || b[pos+1] != 0x8b || b[pos+2] != 8 {
			return gzip.ErrHeader
		}
		flg := b[pos+3]
		pos += 10
		if flg&0x04 != 0 { // FEXTRA
			if len(b)-pos < 2 {
				return io.ErrUnexpectedEOF
			}
			n := le16(pos)
			pos += 2
			if len(b)-pos < n {
				return io.ErrUnexpectedEOF
			}
			pos += n
		}
		for _, f := range [2]byte{0x08, 0x10} { // FNAME, FCOMMENT: at most 511 bytes and a NUL
			if flg&f == 0 {
				continue
			}
			for i := 0; ; i++ {
				if i >= 512 {
					return gzip.ErrHeader
				}
				if pos >= len(b) {
					return io.ErrUnexpectedEOF
				}
				pos++
				if b[pos-1] == 0 {
					break
				}
			}
		}
		if flg&0x02 != 0 { // FHCRC
			if len(b)-pos < 2 {
				return io.ErrUnexpectedEOF
			}
			if le16(pos) != int(uint16(crc32.ChecksumIEEE(b[start:pos]))) {
				return gzip.ErrHeader
			}
			pos += 2
		}
		base, bits, nbit = len(out), 0, 0
		for last := 0; last == 0; {
			var err error
			if last, err = need(1); err != nil {
				return err
			}
			typ, err := need(2)
			if err != nil {
				return err
			}
			switch typ {
			case 0:
				err = stored()
			case 1:
				err = fixed()
			case 2:
				err = dynamic()
			default:
				err = corrupt()
			}
			if err != nil {
				return err
			}
		}
		if len(b)-pos < 8 {
			return io.ErrUnexpectedEOF
		}
		if le32(pos) != crc32.ChecksumIEEE(out[base:]) || le32(pos+4) != uint32(len(out)-base) {
			return gzip.ErrChecksum
		}
		pos += 8
		return nil
	}
	// b is not empty (&b[0] above). The members after the first are read
	// until it ends.
	for pos < len(b) {
		if err := member(); err != nil {
			panic(err)
		}
	}
	b2 := out

	fd := filedesc.Builder{
		RawDescriptor: b2,
		FileRegistry:  resolverOnly{protoregistry.GlobalFiles}, // do not register back to global registry
	}.Build().File
	if fd, ok := legacyFileDescCache.LoadOrStore(&b[0], fd); ok {
		return fd.(protoreflect.FileDescriptor)
	}
	return fd
}
