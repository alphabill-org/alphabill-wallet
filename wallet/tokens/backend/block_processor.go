package backend

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/broker"
)

type blockProcessor struct {
	store  Storage
	notify func(bearerPredicate []byte, msg broker.Message)
	log    *slog.Logger
}

func (p *blockProcessor) ProcessBlock(ctx context.Context, b *types.Block) error {
	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number: %w", err)
	}
	// block numbers must not be sequential (gaps might appear as empty block are not stored
	// and sent) but must be in ascending order
	if lastBlockNumber >= b.UnicityCertificate.InputRecord.RoundNumber {
		return fmt.Errorf("invalid block, received block %d, current wallet block %d", b.UnicityCertificate.InputRecord.RoundNumber, lastBlockNumber)
	}

	for idx, tx := range b.Transactions {
		proof, _, err := types.NewTxProof(b, idx, crypto.SHA256)
		if err != nil {
			return fmt.Errorf("failed to create tx proof for the block: %w", err)
		}
		if err := p.processTx(tx, proof); err != nil {
			return fmt.Errorf("failed to process tx: %w", err)
		}
	}

	return p.store.SetBlockNumber(b.GetRoundNumber())
}

func (p *blockProcessor) processTx(tr *types.TransactionRecord, proof *wallet.TxProof) error {
	var err error
	tx := tr.TransactionOrder
	id := tx.UnitID()
	txProof := &wallet.Proof{TxRecord: tr, TxProof: proof}
	txHash := tx.Hash(crypto.SHA256)
	p.log.Debug(fmt.Sprintf("process %s transaction", tx.PayloadType()), logger.UnitID(id))

	// handle UT transactions
	switch tx.Payload.Type {
	case tokens.PayloadTypeCreateFungibleTokenType:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.CreateFungibleTokenTypeAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		return p.saveTokenType(&TokenUnitType{
			Kind:                     Fungible,
			ID:                       id,
			ParentTypeID:             attrs.ParentTypeID,
			Symbol:                   attrs.Symbol,
			Name:                     attrs.Name,
			Icon:                     attrs.Icon,
			DecimalPlaces:            attrs.DecimalPlaces,
			SubTypeCreationPredicate: attrs.SubTypeCreationPredicate,
			TokenCreationPredicate:   attrs.TokenCreationPredicate,
			InvariantPredicate:       attrs.InvariantPredicate,
			TxHash:                   txHash,
		}, txProof)
	case tokens.PayloadTypeMintFungibleToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.MintFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		tokenType, err := p.store.GetTokenType(attrs.TypeID)
		if err != nil {
			return fmt.Errorf("mint fungible token tx: failed to get token type with id=%X, token id=%X: %w", attrs.TypeID, id, err)
		}
		return p.saveToken(
			&TokenUnit{
				ID:       id,
				TypeID:   attrs.TypeID,
				TypeName: tokenType.Name,
				Amount:   attrs.Value,
				Kind:     tokenType.Kind,
				Symbol:   tokenType.Symbol,
				Decimals: tokenType.DecimalPlaces,
				TxHash:   txHash,
				Owner:    attrs.Bearer,
			},
			txProof)
	case tokens.PayloadTypeTransferFungibleToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.TransferFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("fungible transfer tx: failed to get token with id=%X: %w", id, err)
		}
		token.TxHash = txHash
		token.Owner = attrs.NewBearer
		return p.saveToken(token, txProof)
	case tokens.PayloadTypeSplitFungibleToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.SplitFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		// check and update existing token
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("split tx: failed to get token with id=%X: %w", id, err)
		}
		if !bytes.Equal(token.TypeID, attrs.TypeID) {
			return fmt.Errorf("split tx: type id does not match (received '%X', expected '%X'), token id=%X", attrs.TypeID, token.TypeID, token.ID)
		}
		remainingValue := token.Amount - attrs.TargetValue
		if attrs.RemainingValue != remainingValue {
			return fmt.Errorf("split tx: invalid remaining amount (received '%v', expected '%v'), token id=%X", attrs.RemainingValue, remainingValue, token.ID)
		}

		token.Amount = remainingValue
		token.TxHash = txHash
		if err = p.saveToken(token, txProof); err != nil {
			return err
		}

		// save new token created by the split
		newToken := &TokenUnit{
			ID:       tokens.NewFungibleTokenID(id, tokens.HashForIDCalculation(tx, crypto.SHA256)),
			Symbol:   token.Symbol,
			TypeID:   token.TypeID,
			TypeName: token.TypeName,
			Kind:     token.Kind,
			Amount:   attrs.TargetValue,
			Decimals: token.Decimals,
			TxHash:   txHash,
			Owner:    attrs.NewBearer,
		}
		return p.saveToken(newToken, txProof)
	case tokens.PayloadTypeBurnFungibleToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.BurnFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		token, err := p.store.GetToken(id)
		if err != nil {
			return err
		}
		if token.Amount != attrs.Value {
			return fmt.Errorf("expected burned amount: %v, got %v. token id='%X', type id='%X'", token.Amount, attrs.Value, token.ID, token.TypeID)
		}
		token.TxHash = txHash
		token.Burned = true
		return p.saveToken(token, txProof)
	case tokens.PayloadTypeJoinFungibleToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.JoinFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		joinedToken, err := p.store.GetToken(id)
		if err != nil {
			return err
		}
		if joinedToken == nil {
			return nil
		}
		burnedTokensToRemove := make([]TokenID, 0, len(attrs.BurnTransactions))
		var burnedValue uint64
		for _, burnTx := range attrs.BurnTransactions {
			burnTxAttr := &tokens.BurnFungibleTokenAttributes{}
			if err = burnTx.TransactionOrder.UnmarshalAttributes(burnTxAttr); err != nil {
				return err
			}
			burnedID := burnTx.TransactionOrder.UnitID()
			burnedToken, err := p.store.GetToken(burnedID)
			if err != nil {
				return err
			}
			if !burnedToken.Burned {
				return fmt.Errorf("token with id '%X' is expected to be burned, but it is not", burnedID)
			}
			if !bytes.Equal(burnedToken.Owner, joinedToken.Owner) {
				return fmt.Errorf("expected burned token's bearer '%X', got %X", joinedToken.Owner, burnedToken.Owner)
			}
			if !bytes.Equal(joinedToken.ID, burnTxAttr.TargetTokenID) {
				return fmt.Errorf("expected burned token's target id '%X', got %X", joinedToken.ID, burnTxAttr.TargetTokenID)
			}
			if !bytes.Equal(joinedToken.TxHash, burnTxAttr.TargetTokenBacklink) {
				return fmt.Errorf("expected burned token's target backlink '%X', got %X", joinedToken.TxHash, burnTxAttr.TargetTokenBacklink)
			}
			burnedTokensToRemove = append(burnedTokensToRemove, burnedID)
			burnedValue += burnTxAttr.Value
		}
		joinedToken.Amount += burnedValue
		joinedToken.TxHash = txHash
		joinedToken.Locked = 0
		if err = p.saveToken(joinedToken, txProof); err != nil {
			return fmt.Errorf("failed to save joined token: %w", err)
		}
		for _, burnedID := range burnedTokensToRemove {
			if err = p.store.RemoveToken(burnedID); err != nil {
				return fmt.Errorf("failed to remove burned token %X: %w", burnedID, err)
			}
		}
		return nil
	case tokens.PayloadTypeCreateNFTType:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.CreateNonFungibleTokenTypeAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		return p.saveTokenType(&TokenUnitType{
			Kind:                     NonFungible,
			ID:                       id,
			ParentTypeID:             attrs.ParentTypeID,
			Symbol:                   attrs.Symbol,
			Name:                     attrs.Name,
			Icon:                     attrs.Icon,
			SubTypeCreationPredicate: attrs.SubTypeCreationPredicate,
			TokenCreationPredicate:   attrs.TokenCreationPredicate,
			InvariantPredicate:       attrs.InvariantPredicate,
			NftDataUpdatePredicate:   attrs.DataUpdatePredicate,
			TxHash:                   txHash,
		}, txProof)
	case tokens.PayloadTypeMintNFT:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.MintNonFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		tokenType, err := p.store.GetTokenType(attrs.NFTTypeID)
		if err != nil {
			return fmt.Errorf("mint nft tx: failed to get token type with id=%X, token id=%X: %w", attrs.NFTTypeID, id, err)
		}

		newToken := &TokenUnit{
			ID:                     id,
			Kind:                   tokenType.Kind,
			TypeID:                 attrs.NFTTypeID,
			TypeName:               tokenType.Name,
			Symbol:                 tokenType.Symbol,
			NftName:                attrs.Name,
			NftURI:                 attrs.URI,
			NftData:                attrs.Data,
			NftDataUpdatePredicate: attrs.DataUpdatePredicate,
			TxHash:                 txHash,
			Owner:                  attrs.Bearer,
		}
		return p.saveToken(newToken, txProof)
	case tokens.PayloadTypeTransferNFT:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.TransferNonFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("transfer nft tx: failed to get token with id=%X: %w", id, err)
		}
		token.Owner = attrs.NewBearer
		token.TxHash = txHash
		return p.saveToken(token, txProof)
	case tokens.PayloadTypeUpdateNFT:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		attrs := &tokens.UpdateNonFungibleTokenAttributes{}
		if err = tx.UnmarshalAttributes(attrs); err != nil {
			return err
		}
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("update nft tx: failed to get token with id=%X: %w", id, err)
		}
		token.NftData = attrs.Data
		token.TxHash = txHash
		return p.saveToken(token, txProof)
	case tokens.PayloadTypeLockToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("lock token tx: failed to get token with id=%X: %w", id, err)
		}
		attr := &tokens.LockTokenAttributes{}
		if err := tx.UnmarshalAttributes(attr); err != nil {
			return err
		}
		token.Locked = attr.LockStatus
		token.TxHash = txHash
		return p.saveToken(token, txProof)
	case tokens.PayloadTypeUnlockToken:
		if err := p.updateFCB(tr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		token, err := p.store.GetToken(id)
		if err != nil {
			return fmt.Errorf("unlock token tx: failed to get token with id=%X: %w", id, err)
		}
		token.Locked = 0
		token.TxHash = txHash
		return p.saveToken(token, txProof)
	case transactions.PayloadTypeAddFeeCredit:
		addFeeCreditAttributes := &transactions.AddFeeCreditAttributes{}
		if err = tx.UnmarshalAttributes(addFeeCreditAttributes); err != nil {
			return err
		}
		transferFeeCreditAttributes := &transactions.TransferFeeCreditAttributes{}
		if err = addFeeCreditAttributes.FeeCreditTransfer.TransactionOrder.UnmarshalAttributes(transferFeeCreditAttributes); err != nil {
			return err
		}
		fcb, err := p.store.GetFeeCreditBill(id)
		if err != nil {
			return err
		}
		return p.store.SetFeeCreditBill(&FeeCreditBill{
			Id:     id,
			Value:  fcb.GetValue() + transferFeeCreditAttributes.Amount - addFeeCreditAttributes.FeeCreditTransfer.ServerMetadata.ActualFee - tr.ServerMetadata.ActualFee,
			TxHash: txHash,
		}, txProof)
	case transactions.PayloadTypeCloseFeeCredit:
		closeFeeCreditAttributes := &transactions.CloseFeeCreditAttributes{}
		if err = tx.UnmarshalAttributes(closeFeeCreditAttributes); err != nil {
			return err
		}
		fcb, err := p.store.GetFeeCreditBill(id)
		if err != nil {
			return err
		}
		return p.store.SetFeeCreditBill(&FeeCreditBill{
			Id:     id,
			Value:  fcb.GetValue() - closeFeeCreditAttributes.Amount,
			TxHash: txHash,
		}, txProof)
	case transactions.PayloadTypeLockFeeCredit:
		fcb, err := p.store.GetFeeCreditBill(tx.UnitID())
		if err != nil {
			return fmt.Errorf("failed to load fee credit bill: %w", err)
		}
		if fcb == nil {
			return fmt.Errorf("fee credit bill not found: %X", tx.GetClientFeeCreditRecordID())
		}
		actualFee := tr.GetActualFee()
		if fcb.Value < actualFee {
			return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, actualFee)
		}
		attr := &transactions.LockFeeCreditAttributes{}
		if err := tx.UnmarshalAttributes(attr); err != nil {
			return fmt.Errorf("failed to unmarshal lockFC attributes: %w", err)
		}
		fcb.Locked = wallet.LockReason(attr.LockStatus)
		fcb.Value -= actualFee
		fcb.TxHash = txHash
		return p.store.SetFeeCreditBill(fcb, txProof)
	case transactions.PayloadTypeUnlockFeeCredit:
		fcb, err := p.store.GetFeeCreditBill(tx.UnitID())
		if err != nil {
			return fmt.Errorf("failed to load fee credit bill: %w", err)
		}
		if fcb == nil {
			return fmt.Errorf("fee credit bill not found: %X", tx.GetClientFeeCreditRecordID())
		}
		actualFee := tr.GetActualFee()
		if fcb.Value < actualFee {
			return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, actualFee)
		}
		fcb.Locked = 0
		fcb.Value -= actualFee
		fcb.TxHash = txHash
		return p.store.SetFeeCreditBill(fcb, txProof)
	default:
		p.log.Error(fmt.Sprintf("received unknown token transaction type %q, skipped processing", tx.Payload.Type), logger.UnitID(id))
		return nil
	}
}

func (p *blockProcessor) saveTokenType(unit *TokenUnitType, proof *wallet.Proof) error {
	if err := p.store.SaveTokenType(unit, proof); err != nil {
		return fmt.Errorf("failed to store token type: %w", err)
	}
	return nil
}

func (p *blockProcessor) saveToken(unit *TokenUnit, proof *wallet.Proof) error {
	if err := p.store.SaveToken(unit, proof); err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}
	p.notify(unit.Owner, unit)
	return nil
}

func (p *blockProcessor) updateFCB(tx *types.TransactionRecord) error {
	fcb, err := p.store.GetFeeCreditBill(tx.TransactionOrder.GetClientFeeCreditRecordID())
	if err != nil {
		return err
	}
	if fcb == nil {
		return fmt.Errorf("fee credit bill not found: %X", tx.TransactionOrder.GetClientFeeCreditRecordID())
	}
	actualFee := tx.GetActualFee()
	if fcb.Value < actualFee {
		return fmt.Errorf("insufficient fee credit - fee is %d but remaining credit is only %d", actualFee, fcb.Value)
	}
	fcb.Value -= actualFee
	return p.store.SetFeeCreditBill(fcb, nil)
}
