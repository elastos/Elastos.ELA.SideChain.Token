package core

import (
	"io"

	"github.com/elastos/Elastos.ELA.SideChain/core"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

func InitOutputHelper() {
	core.OutputHelper = &core.OutputHelperBase{}
	core.OutputHelper.Init()
	core.OutputHelper.Serialize = SerializeOutput
	core.OutputHelper.Deserialize = DeserializeOutput
}

func SerializeOutput(output *core.Output, w io.Writer) error {
	err := output.AssetID.Serialize(w)
	if err != nil {
		return err
	}

	if output.AssetID.IsEqual(core.GetSystemAssetId()) {
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

func DeserializeOutput(output *core.Output, r io.Reader) error {
	err := output.AssetID.Deserialize(r)
	if err != nil {
		return err
	}

	if output.AssetID.IsEqual(core.GetSystemAssetId()) {
		err = output.Value.Deserialize(r)
		if err != nil {
			return err
		}
	} else {
		bytes, err := ReadVarBytes(r)
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
