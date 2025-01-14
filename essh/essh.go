package essh

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var LayerTypeESSH gopacket.LayerType

// ESSHType defines the type of data afdter the ESSH Record
type ESSHType uint8

// ESSHType known values, possibly defined in RFC 4253, section 12.
const (
	ESSH_BANNER         ESSHType = 53
	ESSH_MSG_KEXINIT    ESSHType = 20 // SSH_MSG_KEXINIT
	ESSH_MSG_NEW_KEYS            = 21 // SSH_MSG_NEWKEYS
	ESSH_MSG_DHKEXINIT  ESSHType = 30
	ESSH_MSG_DHKEXREPLY ESSHType = 31
)

// String shows the register type nicely formatted
func (ss ESSHType) String() string {
	switch ss {
	default:
		return "Unknown"
	case ESSH_BANNER:
		return "Banner"
	case ESSH_MSG_KEXINIT:
		return "Key Exchange Init"
	case ESSH_MSG_NEW_KEYS:
		return "New Keys"
	case ESSH_MSG_DHKEXINIT:
		return "Diffie-Hellman Key Exchange Init"
	case ESSH_MSG_DHKEXREPLY:
		return "Diffie-Hellman Key Exchange Reploy"

	}
}

// ESSHVersion represents the ESSH version in numeric format
type ESSHVersion uint16

// Strings shows the ESSH version nicely formatted
func (sv ESSHVersion) String() string {
	switch sv {
	default:
		return "Unknown"
	}
}

// SSH is specified in RFC 4253

type ESSH struct {
	layers.BaseLayer

	BannersComplete bool

	// ESSH Records
	Banner  *ESSHBannerRecord
	Kexinit *ESSHKexinitRecord
}

// decodeFromBytes decodes the Binary Packet Protocol as specified by RFC 4253, section 6.
//
//   uint32     packet_length
//   byte       padding_length
//   byte       message code
type ESSHRecordHeader struct {
	PacketLength  uint32
	PaddingLength uint8
	MessageCode   ESSHType
}

func (h *ESSHRecordHeader) decodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	if len(data) < 6 {
		return errors.New("ESSH invalid SSH header")
	}
	h.PacketLength = binary.BigEndian.Uint32(data[0:4])
	h.PaddingLength = uint8(data[4:5][0])
	h.MessageCode = ESSHType(uint8(data[5:6][0]))
	return nil
}

func NewESSH(decb bool) *ESSH {
	return &ESSH{
		BannersComplete: decb,
	}
}

func (s *ESSH) LayerType() gopacket.LayerType { return LayerTypeESSH }

// decodeESSH decodes the byte slice into a ESSH type. IT also setups
// the application Layer in PacketBuilder.
func decodeESSH(data []byte, p gopacket.PacketBuilder) error {
	s := &ESSH{}
	err := s.DecodeFromBytes(data, p)
	if err != nil {
		return err
	}
	p.AddLayer(s)
	p.SetApplicationLayer(s)
	return nil
}

// DecodeFromBytes decodes a byte slice into the ESSH struct
func (s *ESSH) DecodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	s.BaseLayer.Contents = data
	s.BaseLayer.Payload = nil
	return s.decodeESSHRecords(data, df)
}

func (s *ESSH) decodeESSHRecords(data []byte, df gopacket.DecodeFeedback) error {
	var err error

	if len(data) < 4 {
		df.SetTruncated()
		return errors.New("ESSH record too short")
	}

	// since there are no further layers, the baselayer's content is
	// pointing to this layer
	s.BaseLayer = layers.BaseLayer{Contents: data[:len(data)]}

	// If banners are not complete, try to parse these first.
	if !s.BannersComplete {
		var r ESSHBannerRecord
		bl, err := r.decodeFromBytes(data, df)
		if err != nil {
			// We must parse banners first, and these banners are invalid. Abort!
			return nil
		}

		// Banner successful!
		s.Banner = &r
		s.BannersComplete = true // important, if we have more data!
		if bl == len(data) {
			// All data is decoded!
			return nil
		}

		// We must decode the rest of the data!
		return s.decodeESSHRecords(data[bl:len(data)], df)
	}

	err = s.decodeKexRecords(data, df)
	if err != nil {
		return err
	}

	return nil
}

func (s *ESSH) decodeKexRecords(data []byte, df gopacket.DecodeFeedback) error {
	var h ESSHRecordHeader
	err := h.decodeFromBytes(data, df)
	if err != nil {
		return err
	}

	hl := 6                            // header length
	tl := hl + int(h.PacketLength) - 2 // minus padding_length and MessageCode field
	if len(data) < tl {
		df.SetTruncated()
		return errors.New("ESSH packet length mismatch")
	}

	if h.MessageCode != ESSH_MSG_KEXINIT {
		return fmt.Errorf("Wrong messagecode (%d), should be ESSH_MSG_KEXINIT (%d)", h.MessageCode, ESSH_MSG_KEXINIT)
	}

	var r ESSHKexinitRecord
	err = r.decodeFromBytes(data[hl:tl], h.PaddingLength, gopacket.NilDecodeFeedback)
	if err != nil {
		return err
	}
	// Key Exchange successful!
	s.Kexinit = &r
	return nil
}

// CanDecode implements gopacket.DecodingLayer.
func (s *ESSH) CanDecode() gopacket.LayerClass {
	return LayerTypeESSH
}

// NextLayerType implements gopacket.DecodingLayer.
func (t *ESSH) NextLayerType() gopacket.LayerType {
	return gopacket.LayerTypeZero
}

func (s *ESSH) Payload() []byte {
	return nil
}

func init() {
	LayerTypeESSH = gopacket.RegisterLayerType(6666, gopacket.LayerTypeMetadata{Name: "ESSH", Decoder: gopacket.DecodeFunc(decodeESSH)})
}
