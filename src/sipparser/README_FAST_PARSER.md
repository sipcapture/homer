# Fast SIP Parser - Ultra-High Performance SIP Parser

## Description

`FastSIPParser` is a super-efficient SIP parser in Go, optimized for maximum performance using:

- **Zero-copy techniques** - using pointers instead of copying data
- **Unsafe pointers** - direct memory access to minimize allocations
- **SIMD-like optimizations** - processing multiple bytes simultaneously
- **Assembly optimizations** - for critical code sections (amd64/arm64)
- **Lazy evaluation** - string conversion only when needed

## Performance

The parser is designed to process millions of SIP messages per second with minimal memory allocations.

### Key Optimizations:

1. **Pointers instead of copying**: All SIP message fields are slices pointing to original data
2. **SIMD-like operations**: Header comparison happens byte-by-byte with 4-8 bytes processed simultaneously
3. **Minimal allocations**: Reusable buffers and pointers are used
4. **Optimized search**: Pattern search (\r\n, headers) uses pointer arithmetic

## Usage

```go
package main

import (
    "fmt"
    "github.com/sipcapture/homer-core/src/sipparser"
)

func main() {
    // Create parser
    parser := sipparser.NewFastSIPParser()
    
    // SIP message as []byte
    sipData := []byte(`INVITE sip:alice@example.com SIP/2.0
Call-ID: test@example.com
From: <sip:alice@example.com>;tag=123
To: <sip:bob@example.com>
Content-Length: 0

`)
    
    // Parse
    msg, err := parser.Parse(sipData)
    if err != nil {
        panic(err)
    }
    
    // Usage (zero-copy access)
    fmt.Printf("Method: %s\n", msg.GetMethod())
    fmt.Printf("Call-ID: %s\n", msg.GetCallID())
    fmt.Printf("From Tag: %s\n", string(msg.FromTag))
    fmt.Printf("To Tag: %s\n", string(msg.ToTag))
}
```

## SIPMessage Structure

All fields are byte slices pointing to original data (zero-copy):

```go
type SIPMessage struct {
    // Raw data
    Data     []byte
    DataPtr  *byte
    
    // Start line (zero-copy)
    Method      []byte
    RequestURI  []byte
    ResponseCode []byte
    ResponseText []byte
    Version     []byte
    
    // Headers (zero-copy)
    CallID      []byte
    From        []byte
    To          []byte
    Via         []byte
    CSeq        []byte
    Contact     []byte
    ContentType []byte
    ContentLen  []byte
    UserAgent   []byte
    
    // Parsed values
    FromTag     []byte
    ToTag       []byte
    Branch      []byte
    
    // Flags
    IsRequest   bool
    IsResponse  bool
    HasBody     bool
    
    // Body
    Body        []byte
    BodyOffset  int
}
```

## Benchmarks

Run benchmarks to compare performance:

```bash
cd src/sipparser
go test -bench=. -benchmem
```

Expected results:
- **FastParser**: ~100-200ns per message, 0-1 allocation
- **OldParser**: ~500-1000ns per message, 5-10 allocations

## Technical Details

### Zero-Copy Approach

Instead of copying strings, the parser uses byte slices that point to original data:

```go
// Instead of: msg.CallID = string(value)  // allocation!
// We use: msg.CallID = value              // zero-copy
```

### SIMD-like Optimizations

Header comparison happens byte-by-byte with multiple bytes processed simultaneously:

```go
// Compare first 4 bytes of header in one operation
first4 := *(*uint32)(unsafe.Pointer(&headerName[0]))
if first4 == uint32('c')|(uint32('a')<<8)|(uint32('l')<<16)|(uint32('l')<<24) {
    // This is Call-ID header
}
```

### Assembly Optimizations

For amd64 and arm64 platforms, optimized pattern search functions are used.

## Limitations

1. **Unsafe code**: Uses `unsafe` package - requires caution
2. **Memory**: Original data must remain in memory while SIPMessage is used
3. **Platforms**: Assembly optimizations available only for amd64 and arm64

## Safety

- All unsafe pointer operations check bounds
- Input data validation before parsing
- Fallback implementations for platforms without assembly

## Comparison with C++

This parser implements the same optimization techniques used in high-performance C++ parsers:

- Direct memory access through pointers
- Minimal allocations
- SIMD-like operations
- CPU cache optimization

## License

GNU Affero General Public License v3.0
