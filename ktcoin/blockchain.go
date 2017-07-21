package ktcoin

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// A Block consists of the previous block's hash, the list of
// transactions it enacts, and a nonce.  For a block to be valid, the
// SHA256 hash must have sufficient leading zeroes to satisfy the
// proof of work property.
type Block struct {
	prevHash     SHA
	nonce        int
	transactions []Transaction
}

type BlockChain struct {
	blocks           []Block
	openTransactions map[SHA]map[string]int
}

func NewBlockChain() BlockChain {
	genesisHash := sha256.Sum256([]byte("genesis"))
	blocks := make([]Block, 0)
	blocks = append(blocks, Block{genesisHash, 0, make([]Transaction, 0)})
	openTransactions := make(map[SHA]map[string]int)
	return BlockChain{
		blocks,
		openTransactions,
	}
}

func (block *Block) Hash() SHA {
	contents := make([]byte, 0)
	contents = append(contents, block.prevHash[:]...)
	nonceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(nonceBytes, uint64(block.nonce))
	contents = append(contents, nonceBytes...)

	for _, t := range block.transactions {
		hashedTransaction := t.Hash()
		contents = append(contents, hashedTransaction[:]...)
	}

	hash := sha256.Sum256(contents)
	return hash
}

func (bc *BlockChain) GetOpenInputs(key rsa.PublicKey) map[SHA]int {
	openInputs := make(map[SHA]int)

	for sha, outputs := range bc.openTransactions {
		amount, isPresent := outputs[publicKeyString(key)]
		if isPresent {
			openInputs[sha] = amount
		}
	}

	return openInputs
}

func (bc *BlockChain) addNextBlock(transactions []Transaction) error {
	// Verify transactions
	for i, t := range transactions {
		if i == 0 {
			// Special case: money from nothing
			outputTotal := 0
			for _, v := range t.Outputs {
				outputTotal += v
			}
			if outputTotal != 25 {
				return errors.New("Invalid genesis transaction: does not create 25 coins")
			}
		} else {
			err := bc.Verify(&t)
			if err != nil {
				fmt.Println("verification error")
				return err
			}
		}
	}

	// Look for the magic hash value
	mostRecentBlock := bc.blocks[len(bc.blocks)-1]
	prevHash := mostRecentBlock.Hash()

	nonce := 0
	newBlock := Block{prevHash, nonce, transactions}
	hashedBlock := newBlock.Hash()

	for hashedBlock[0] != 0 {
		newBlock.nonce++
		hashedBlock = newBlock.Hash()
	}

	// Append the block to the chain
	bc.blocks = append(bc.blocks, newBlock)
	fmt.Println(len(bc.blocks))

	for _, transaction := range transactions {
		for _, input := range transaction.Inputs {
			delete(bc.openTransactions[input], publicKeyString(transaction.Sender))
		}
		hashedTransaction := transaction.Hash()
		bc.openTransactions[hashedTransaction] = transaction.Outputs
	}
	return nil
}

// How to verify a transaction on the block chain:
// - Check that the
//   transaction is internally consistent (inputs equal outputs,
//   signature is valid)
// - Check that each of the transaction's inputs
//   is open for spending (i.e. hasn't been used yet as an input to
//   another transaction)

// How to store information on the block chain? Keep a set of transactions open for spending?
func (bc *BlockChain) Verify(t *Transaction) error {
	// Verify signature
	hashed, err := bytesToSign(t.Recipient, t.Inputs)
	if err != nil {
		return err
	}
	err = rsa.VerifyPKCS1v15(&t.Sender, crypto.SHA256, hashed[:], t.Signature)
	if err != nil {
		return errors.New("invalid signature")
	}

	// Verify tx inputs are keys in t.openTransactions
	for _, input := range t.Inputs {
		if val, ok := bc.openTransactions[input]; ok {
			if _, ok = val[publicKeyString(t.Sender)]; !ok {
				return errors.New("Sender does not own this transaction")
			}
		} else {
			return errors.New("Transaction not open")
		}
	}

	// Verify tx amounts are valid (inputs equal outputs)
	inputTotal := 0
	for _, inputSha := range t.Inputs {
		outputAmounts, _ := bc.openTransactions[inputSha]
		senderAmount, _ := outputAmounts[publicKeyString(t.Sender)]
		inputTotal += senderAmount
	}

	outputTotal := 0
	for _, amount := range t.Outputs {
		outputTotal += amount
	}

	if inputTotal != outputTotal {
		return errors.New("tx inputs do not match outputs")
	}

	return nil
}