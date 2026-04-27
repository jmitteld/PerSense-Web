object MessageDialog: TMessageDialog
  Left = 430
  Top = 381
  BorderStyle = bsDialog
  Caption = 'Message'
  ClientHeight = 323
  ClientWidth = 323
  Color = clBtnFace
  ParentFont = True
  OldCreateOrder = True
  Position = poScreenCenter
  PixelsPerInch = 96
  TextHeight = 13
  object MessageText: TLabel
    Left = 16
    Top = 16
    Width = 64
    Height = 13
    Caption = 'MessageText'
    WordWrap = True
  end
  object OKBtn: TButton
    Left = 7
    Top = 100
    Width = 66
    Height = 25
    Caption = 'OK'
    Default = True
    ModalResult = 1
    TabOrder = 0
  end
  object CancelBtn: TButton
    Left = 167
    Top = 100
    Width = 66
    Height = 25
    Cancel = True
    Caption = 'Cancel'
    ModalResult = 2
    TabOrder = 1
  end
  object DetailsBtn: TButton
    Left = 248
    Top = 100
    Width = 67
    Height = 25
    Caption = 'Details >>'
    TabOrder = 2
    OnClick = DetailsBtnClick
  end
  object HelpMemo: TMemo
    Left = 8
    Top = 136
    Width = 313
    Height = 185
    Lines.Strings = (
      'HelpMemo')
    ScrollBars = ssVertical
    TabOrder = 3
  end
  object NoBtn: TButton
    Left = 88
    Top = 100
    Width = 65
    Height = 25
    Caption = 'No'
    ModalResult = 7
    TabOrder = 4
  end
end
