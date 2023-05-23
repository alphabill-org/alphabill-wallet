package backend

import (
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

const DustBillDeletionTimeout = 65536

type BlockProcessor struct {
	store       BillStore
	TxConverter block.TxConverter
	sdrs        map[string]*genesis.SystemDescriptionRecord
	moneySDR    *genesis.SystemDescriptionRecord
}

func NewBlockProcessor(store BillStore, txConverter block.TxConverter, moneySystemID []byte) (*BlockProcessor, error) {
	sdrs, err := store.Do().GetSystemDescriptionRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to get system description records: %w", err)
	}
	sdrsMap := map[string]*genesis.SystemDescriptionRecord{}
	for _, sdr := range sdrs {
		sdrsMap[string(sdr.SystemIdentifier)] = sdr
	}
	return &BlockProcessor{store: store, TxConverter: txConverter, sdrs: sdrsMap, moneySDR: sdrsMap[string(moneySystemID)]}, nil
}

func (p *BlockProcessor) ProcessBlock(_ context.Context, b *block.Block) error {
	roundNumber := b.GetRoundNumber()
	wlog.Info("processing block: ", roundNumber)
	return p.store.WithTransaction(func(dbTx BillStoreTx) error {
		lastBlockNumber, err := dbTx.GetBlockNumber()
		if err != nil {
			return err
		}
		if lastBlockNumber >= roundNumber {
			return fmt.Errorf("invalid block number. Received blockNumber %d current wallet blockNumber %d", roundNumber, lastBlockNumber)
		}
		genericBlock, err := b.ToGenericBlock(p.TxConverter)
		if err != nil {
			return fmt.Errorf("failed to convert block to generic block: %w", err)
		}
		for _, tx := range genericBlock.Transactions {
			if err := p.processTx(tx, genericBlock, dbTx); err != nil {
				return fmt.Errorf("failed to process transaction: %w", err)
			}
		}
		if err := dbTx.DeleteExpiredBills(roundNumber); err != nil {
			return fmt.Errorf("failed to delete expired bills: %w", err)
		}
		return dbTx.SetBlockNumber(roundNumber)
	})
}

func (p *BlockProcessor) processTx(gtx txsystem.GenericTransaction, b *block.GenericBlock, dbTx BillStoreTx) error {
	txPb := gtx.ToProtoBuf()
	roundNumber := b.GetRoundNumber()
	switch tx := gtx.(type) {
	case moneytx.Transfer:
		wlog.Info(fmt.Sprintf("received transfer order (UnitID=%x)", txPb.UnitId))
		err := p.updateFCB(txPb, roundNumber, dbTx)
		if err != nil {
			return err
		}
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             txPb.UnitId,
			Value:          tx.TargetValue(),
			TxHash:         tx.Hash(crypto.SHA256),
			OwnerPredicate: tx.NewBearer(),
		})
		if err != nil {
			return err
		}
	case moneytx.TransferDC:
		wlog.Info(fmt.Sprintf("received TransferDC order (UnitID=%x)", txPb.UnitId))
		err := p.updateFCB(txPb, roundNumber, dbTx)
		if err != nil {
			return err
		}
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             txPb.UnitId,
			Value:          tx.TargetValue(),
			TxHash:         tx.Hash(crypto.SHA256),
			IsDCBill:       true,
			OwnerPredicate: tx.TargetBearer(),
		})
		if err != nil {
			return err
		}
		err = dbTx.SetBillExpirationTime(roundNumber+DustBillDeletionTimeout, txPb.UnitId)
		if err != nil {
			return err
		}
	case moneytx.Split:
		err := p.updateFCB(txPb, roundNumber, dbTx)
		if err != nil {
			return err
		}
		// old bill
		oldBill, err := dbTx.GetBill(txPb.UnitId)
		if err != nil {
			return err
		}
		if oldBill != nil {
			wlog.Info(fmt.Sprintf("received split order (existing UnitID=%x)", txPb.UnitId))
			err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
				Id:             txPb.UnitId,
				Value:          tx.RemainingValue(),
				TxHash:         tx.Hash(crypto.SHA256),
				OwnerPredicate: oldBill.OwnerPredicate,
			})
			if err != nil {
				return err
			}
		} else {
			// we should always have the "previous bill" other than splitting the initial bill or some error condition
			wlog.Warning(fmt.Sprintf("received split order where existing unit was not found, ignoring tx (unitID=%x)", txPb.UnitId))
		}

		// new bill
		newID := utiltx.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
		wlog.Info(fmt.Sprintf("received split order (new UnitID=%x)", newID))
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             util.Uint256ToBytes(newID),
			Value:          tx.Amount(),
			TxHash:         tx.Hash(crypto.SHA256),
			OwnerPredicate: tx.TargetBearer(),
		})
		if err != nil {
			return err
		}
	case moneytx.SwapDC:
		err := p.updateFCB(txPb, roundNumber, dbTx)
		if err != nil {
			return err
		}
		wlog.Info(fmt.Sprintf("received swap order (UnitID=%x)", txPb.UnitId))
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             txPb.UnitId,
			Value:          tx.TargetValue(),
			TxHash:         tx.Hash(crypto.SHA256),
			OwnerPredicate: tx.OwnerCondition(),
		})
		if err != nil {
			return err
		}
		for _, dustTransfer := range tx.DCTransfers() {
			err := dbTx.RemoveBill(util.Uint256ToBytes(dustTransfer.UnitID()))
			if err != nil {
				return err
			}
		}
	case *transactions.TransferFeeCreditWrapper:
		wlog.Info(fmt.Sprintf("received transferFC order (UnitID=%x)", txPb.UnitId))
		bill, err := dbTx.GetBill(txPb.UnitId)
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("unit not found for transferFC tx (unitID=%X)", txPb.UnitId)
		}
		v := tx.TransferFC.Amount + tx.Wrapper.Transaction.ServerMetadata.Fee
		if v < bill.Value {
			bill.Value -= v
			bill.TxHash = tx.Hash(crypto.SHA256)
			err = p.saveBillWithProof(b, txPb, dbTx, bill)
			if err != nil {
				return err
			}
		} else {
			err = dbTx.RemoveBill(bill.Id)
			if err != nil {
				return err
			}
		}
		err = p.addTransferredCreditToPartitionFeeBill(tx, dbTx)
		if err != nil {
			return err
		}
		return p.addTxFeesToMoneyFeeBill(dbTx, txPb)
	case *transactions.AddFeeCreditWrapper:
		wlog.Info(fmt.Sprintf("received addFC order (UnitID=%x)", txPb.UnitId))
		fcb, err := dbTx.GetFeeCreditBill(txPb.UnitId)
		if err != nil {
			return err
		}
		return p.saveFCBWithProof(b, txPb, dbTx, &Bill{
			Id:            txPb.UnitId,
			Value:         fcb.getValue() + tx.TransferFC.TransferFC.Amount - tx.Transaction.ServerMetadata.Fee,
			TxHash:        tx.Hash(crypto.SHA256),
			FCBlockNumber: roundNumber,
		})
	case *transactions.CloseFeeCreditWrapper:
		wlog.Info(fmt.Sprintf("received closeFC order (UnitID=%x)", txPb.UnitId))
		fcb, err := dbTx.GetFeeCreditBill(txPb.UnitId)
		if err != nil {
			return err
		}
		return p.saveFCBWithProof(b, txPb, dbTx, &Bill{
			Id:            txPb.UnitId,
			Value:         fcb.getValue() - tx.CloseFC.Amount,
			TxHash:        tx.Hash(crypto.SHA256),
			FCBlockNumber: roundNumber,
		})
	case *transactions.ReclaimFeeCreditWrapper:
		wlog.Info(fmt.Sprintf("received reclaimFC order (UnitID=%x)", txPb.UnitId))
		bill, err := dbTx.GetBill(txPb.UnitId)
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("unit not found for reclaimFC tx (unitID=%X)", txPb.UnitId)
		}
		reclaimedValue := tx.CloseFCTransfer.CloseFC.Amount - tx.CloseFCTransfer.Transaction.ServerMetadata.Fee - tx.Transaction.ServerMetadata.Fee
		bill.Value += reclaimedValue
		bill.TxHash = tx.Hash(crypto.SHA256)
		err = p.saveBillWithProof(b, txPb, dbTx, bill)
		if err != nil {
			return err
		}
		err = p.removeReclaimedCreditFromPartitionFeeBill(tx, dbTx)
		if err != nil {
			return err
		}
		// add closeFC and reclaimFC txs fees to money partition fee bill
		return p.addTxFeesToMoneyFeeBill(dbTx, tx.CloseFCTransfer.Transaction, txPb)
	default:
		wlog.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", tx))
		return nil
	}
	return nil
}

func (p *BlockProcessor) addTransferredCreditToPartitionFeeBill(tx *transactions.TransferFeeCreditWrapper, dbTx BillStoreTx) error {
	sdr, f := p.sdrs[string(tx.TransferFC.TargetSystemIdentifier)]
	if !f {
		return fmt.Errorf("received transferFC for unknown tx system: %x", tx.TransferFC.TargetSystemIdentifier)
	}
	partitionFeeBill, err := dbTx.GetBill(sdr.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	if partitionFeeBill == nil {
		return fmt.Errorf("partition fee bill not found: %x", sdr.FeeCreditBill.UnitId)
	}
	partitionFeeBill.Value += tx.TransferFC.Amount
	return dbTx.SetBill(partitionFeeBill)
}

func (p *BlockProcessor) removeReclaimedCreditFromPartitionFeeBill(tx *transactions.ReclaimFeeCreditWrapper, dbTx BillStoreTx) error {
	sdr, f := p.sdrs[string(tx.CloseFCTransfer.SystemID())]
	if !f {
		return fmt.Errorf("received reclaimFC for unknown tx system: %x", tx.CloseFCTransfer.SystemID())
	}
	partitionFeeBill, err := dbTx.GetBill(sdr.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	partitionFeeBill.Value -= tx.CloseFCTransfer.CloseFC.Amount
	return dbTx.SetBill(partitionFeeBill)
}

func (p *BlockProcessor) addTxFeesToMoneyFeeBill(dbTx BillStoreTx, txPb ...*txsystem.Transaction) error {
	moneyFeeBill, err := dbTx.GetBill(p.moneySDR.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	for _, tx := range txPb {
		moneyFeeBill.Value += tx.ServerMetadata.Fee
	}
	return dbTx.SetBill(moneyFeeBill)
}

func (p *BlockProcessor) saveBillWithProof(b *block.GenericBlock, tx *txsystem.Transaction, dbTx BillStoreTx, bill *Bill) error {
	err := bill.addProof(b, tx)
	if err != nil {
		return err
	}
	return dbTx.SetBill(bill)
}

func (p *BlockProcessor) saveFCBWithProof(b *block.GenericBlock, tx *txsystem.Transaction, dbTx BillStoreTx, fcb *Bill) error {
	err := fcb.addProof(b, tx)
	if err != nil {
		return err
	}
	return dbTx.SetFeeCreditBill(fcb)
}

func (p *BlockProcessor) updateFCB(tx *txsystem.Transaction, roundNumber uint64, dbTx BillStoreTx) error {
	fcb, err := dbTx.GetFeeCreditBill(tx.ClientMetadata.FeeCreditRecordId)
	if err != nil {
		return err
	}
	if fcb == nil {
		return fmt.Errorf("fee credit bill not found: %X", tx.ClientMetadata.FeeCreditRecordId)
	}
	if fcb.Value < tx.ServerMetadata.Fee {
		return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, tx.ServerMetadata.Fee)
	}
	fcb.Value -= tx.ServerMetadata.Fee
	fcb.FCBlockNumber = roundNumber
	return dbTx.SetFeeCreditBill(fcb)
}