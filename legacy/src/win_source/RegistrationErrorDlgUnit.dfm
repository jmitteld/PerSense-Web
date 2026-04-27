object RegistrationErrorDlg: TRegistrationErrorDlg
  Left = 245
  Top = 108
  BorderStyle = bsDialog
  Caption = 'Registration Error'
  ClientHeight = 151
  ClientWidth = 313
  Color = clBtnFace
  ParentFont = True
  OldCreateOrder = True
  Position = poScreenCenter
  PixelsPerInch = 96
  TextHeight = 13
  object Bevel1: TBevel
    Left = 8
    Top = 8
    Width = 297
    Height = 97
    Shape = bsFrame
  end
  object Label1: TLabel
    Left = 16
    Top = 16
    Width = 281
    Height = 41
    Caption = 
      'The action you have chosen can not be completed with an unregist' +
      'ered version of Per%Sense.  '
    WordWrap = True
  end
  object Label2: TLabel
    Left = 16
    Top = 56
    Width = 281
    Height = 33
    Caption = 'To register Per%Sense now, press the OK button'
    WordWrap = True
  end
  object OKBtn: TButton
    Left = 79
    Top = 116
    Width = 75
    Height = 25
    Caption = 'OK'
    Default = True
    ModalResult = 1
    TabOrder = 0
    OnClick = OKBtnClick
  end
  object CancelBtn: TButton
    Left = 167
    Top = 116
    Width = 75
    Height = 25
    Cancel = True
    Caption = 'Cancel'
    ModalResult = 2
    TabOrder = 1
  end
end
