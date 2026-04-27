object RegistrationDialog: TRegistrationDialog
  Left = 763
  Top = 141
  BorderIcons = [biMinimize, biMaximize]
  BorderStyle = bsDialog
  Caption = 'Registration Information'
  ClientHeight = 210
  ClientWidth = 234
  Color = clBtnFace
  ParentFont = True
  OldCreateOrder = True
  Position = poScreenCenter
  PixelsPerInch = 96
  TextHeight = 13
  object Bevel1: TBevel
    Left = 8
    Top = 8
    Width = 217
    Height = 161
    Shape = bsFrame
  end
  object Label1: TLabel
    Left = 16
    Top = 32
    Width = 201
    Height = 13
    Alignment = taCenter
    Caption = 'Please enter your registration number here:'
    WordWrap = True
  end
  object Label2: TLabel
    Left = 16
    Top = 64
    Width = 201
    Height = 13
    Alignment = taCenter
    Caption = 'Label2'
    WordWrap = True
  end
  object Label3: TLabel
    Left = 16
    Top = 96
    Width = 201
    Height = 13
    Alignment = taCenter
    Caption = 'Label3'
    WordWrap = True
  end
  object OKBtn: TButton
    Left = 60
    Top = 176
    Width = 75
    Height = 25
    Caption = 'OK'
    Default = True
    ModalResult = 1
    TabOrder = 0
  end
  object CancelBtn: TButton
    Left = 148
    Top = 176
    Width = 75
    Height = 25
    Cancel = True
    Caption = 'Cancel'
    ModalResult = 2
    TabOrder = 1
  end
  object Edit1: TEdit
    Left = 16
    Top = 128
    Width = 201
    Height = 21
    TabOrder = 2
    OnKeyUp = Edit1KeyUp
  end
end
