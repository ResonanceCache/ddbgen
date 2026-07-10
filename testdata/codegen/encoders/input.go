// Package encoders covers the code-generation branches the ecommerce
// fixture does not: every encoder in key positions, byte-array and
// unsigned sources, and the valued sort-key condition kinds.
package encoders

import "time"

//ddb:entity table=enc type=sample
//ddb:key pk="S#{ID:ulid}" sk="V#{Ver2:pad6}#{Sum:hex}"
//ddb:index name=ByStamp pk="D#{Digest:hex}" sk="{StampMS:epochms}"
//ddb:pattern name=SamplesEq index=main pk="S#{ID:ulid}" sk.eq="V#{Ver2:pad6}#{Sum:hex}"
//ddb:pattern name=SamplesFrom index=main pk="S#{ID:ulid}" sk.gte="V#{Ver2:pad6}"
//ddb:pattern name=SamplesUntil index=main pk="S#{ID:ulid}" sk.lt="V#{Ver2:pad6}"
//ddb:pattern name=SamplesAbove index=main pk="S#{ID:ulid}" sk.gt="V#{Ver2:pad6}"
//ddb:pattern name=ByStamp index=ByStamp pk="D#{Digest:hex}"
type Sample struct {
	ID      string    `dynamodbav:"id"`
	Ver2    uint32    `dynamodbav:"ver2"`
	Sum     [8]byte   `dynamodbav:"sum"`
	Digest  [16]byte  `dynamodbav:"digest"`
	StampMS int64     `dynamodbav:"stamp_ms"`
	When    time.Time `dynamodbav:"when"`
	Blob    []byte    `dynamodbav:"blob"`
	Label   string    `dynamodbav:"label"`
}

//ddb:entity table=enc type=escaped
//ddb:key pk="E#{Raw:urlenc}" sk="B#{Blob:hex}"
type Escaped struct {
	Raw  string `dynamodbav:"raw"`
	Blob []byte `dynamodbav:"blob"`
}
