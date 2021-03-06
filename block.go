package iolang

import (
	"bytes"
	"strings"
)

type Block struct {
	Object
	Message     *Message
	Self        Interface
	ArgNames    []string
	Activatable bool
}

func (b *Block) Activate(vm *VM, target, locals Interface, msg *Message) Interface {
	// If this block isn't actually activatable, then it should be the result
	// of evaluation.
	if !b.Activatable {
		return b
	}
	return b.reallyActivate(vm, target, locals, msg)
}

func (b *Block) reallyActivate(vm *VM, target, locals Interface, msg *Message) Interface {
	blkLocals := &Object{Slots: Slots{}, Protos: []Interface{vm.BaseObject}}
	for i, arg := range b.ArgNames {
		if x := msg.ArgAt(i); x != nil {
			SetSlot(blkLocals, arg, x)
		} else {
			SetSlot(blkLocals, arg, vm.Nil)
		}
	}
	scope := b.Self
	if scope == nil {
		scope = target
	}
	call := vm.NewCall(locals, b, msg, target)
	SetSlot(blkLocals, "self", scope)
	SetSlot(blkLocals, "call", call)
	result := b.Message.Eval(vm, blkLocals)
	if stop, ok := result.(Stop); ok {
		return stop.Result
	}
	return result
}

func (vm *VM) initBlock() {
	slots := Slots{
		"asString": vm.NewCFunction(BlockAsString, "BlockAsString()"),
		"call":     vm.NewCFunction(BlockCall, "BlockCall(...)"),
	}
	vm.DefaultSlots["Block"] = slots
}

func ObjectBlock(vm *VM, target, locals Interface, msg *Message) Interface {
	blk := Block{
		Object:   Object{Slots: vm.DefaultSlots["Block"], Protos: []Interface{vm.BaseObject}},
		Message:  msg.ArgAt(len(msg.Args) - 1),
		Self:     locals,
		ArgNames: make([]string, len(msg.Args)-1),
	}
	for i, arg := range msg.Args[:len(msg.Args)-1] {
		blk.ArgNames[i] = arg.Name()
	}
	return &blk
}

func ObjectMethod(vm *VM, target, locals Interface, msg *Message) Interface {
	blk := ObjectBlock(vm, target, locals, msg).(*Block)
	blk.Activatable = true
	blk.Self = nil
	return blk
}

func BlockAsString(vm *VM, target, locals Interface, msg *Message) Interface {
	blk := target.(*Block)
	b := bytes.Buffer{}
	if blk.Self == nil {
		b.WriteString("method(")
	} else {
		b.WriteString("block(")
	}
	b.WriteString(strings.Join(blk.ArgNames, ", "))
	if len(blk.ArgNames) > 0 {
		b.WriteByte(',')
	}
	b.WriteByte('\n')
	blk.Message.stringRecurse(vm, &b)
	b.WriteString("\n)")
	return vm.NewString(b.String())
}

func BlockCall(vm *VM, target, locals Interface, msg *Message) Interface {
	return target.(*Block).reallyActivate(vm, target, locals, msg)
}
