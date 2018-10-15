package core

import (
	"io"

	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

const (
	// MaxTokenValueDataSize is the maximum allowed length of token value data.
	MaxTokenValueDataSize = 1024 // 1MB
)

func serializeOutput(output *types.Output, w io.Writer) error {
	err := output.AssetID.Serialize(w)
	if err != nil {
		return err
	}

	if output.AssetID.IsEqual(types.GetSystemAssetId()) {
		err = output.Value.Serialize(w)
		if err != nil {
			return err
		}
	} else {
		err = WriteVarBytes(w, output.TokenValue.Bytes())
		if err != nil {
			return err
		}
	}

	WriteUint32(w, output.OutputLock)

	err = output.ProgramHash.Serialize(w)
	if err != nil {
		return err
	}

	return nil
}

func deserializeOutput(output *types.Output, r io.Reader) error {
	err := output.AssetID.Deserialize(r)
	if err != nil {
		return err
	}

	if output.AssetID.IsEqual(types.GetSystemAssetId()) {
		err = output.Value.Deserialize(r)
		if err != nil {
			return err
		}
	} else {
		bytes, err := ReadVarBytes(r, MaxTokenValueDataSize, "TokenValue")
		if err != nil {
			return err
		}
		output.TokenValue.SetBytes(bytes)
	}

	temp, err := ReadUint32(r)
	if err != nil {
		return err
	}
	output.OutputLock = uint32(temp)

	err = output.ProgramHash.Deserialize(r)
	if err != nil {
		return err
	}

	return nil
}

func Init() {
	types.SerializeOutput = serializeOutput
	types.DeserializeOutput = deserializeOutput
}
