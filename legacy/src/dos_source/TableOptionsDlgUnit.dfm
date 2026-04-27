object TableOptionsDlg: TTableOptionsDlg
  Left = 245
  Top = 108
  BorderStyle = bsDialog
  Caption = 'Output Options for Table'
  ClientHeight = 182
  ClientWidth = 354
  Color = clBtnFace
  ParentFont = True
  OldCreateOrder = True
  Position = poScreenCenter
  PixelsPerInch = 96
  TextHeight = 13
  object Bevel1: TBevel
    Left = 8
    Top = 8
    Width = 337
    Height = 121
    Shape = bsFrame
  end
  object Label1: TLabel
    Left = 32
    Top = 24
    Width = 121
    Height = 13
    Caption = 'Detail lines and Summary:'
  end
  object Label2: TLabel
    Left = 72
    Top = 48
    Width = 78
    Height = 13
    Caption = 'Summary period:'
  end
  object Label3: TLabel
    Left = 24
    Top = 72
    Width = 128
    Height = 13
    Caption = 'Summary in which months?'
  end
  object Label4: TLabel
    Left = 104
    Top = 96
    Width = 47
    Height = 13
    Caption = 'Output to:'
  end
  object OKBtn: TButton
    Left = 183
    Top = 148
    Width = 75
    Height = 25
    Caption = 'OK'
    Default = True
    ModalResult = 1
    TabOrder = 0
    OnClick = OKBtnClick
  end
  object CancelBtn: TButton
    Left = 271
    Top = 148
    Width = 75
    Height = 25
    Cancel = True
    Caption = 'Cancel'
    ModalResult = 2
    TabOrder = 1
  end
  object DetailsCombo: TComboBox
    Left = 160
    Top = 24
    Width = 145
    Height = 21
    Style = csDropDownList
    ItemHeight = 13
    TabOrder = 2
    OnChange = DetailsComboChange
    Items.Strings = (
      'Detail Only'
      'Summary Only'
      'Detail & Summary')
  end
  object SummaryCombo: TComboBox
    Left = 160
    Top = 48
    Width = 145
    Height = 21
    Style = csDropDownList
    ItemHeight = 13
    TabOrder = 3
    OnChange = SummaryComboChange
  end
  object MonthsCombo: TComboBox
    Left = 160
    Top = 72
    Width = 145
    Height = 21
    Style = csDropDownList
    ItemHeight = 13
    TabOrder = 4
  end
  object OutputCombo: TComboBox
    Left = 160
    Top = 96
    Width = 145
    Height = 21
    Style = csDropDownList
    ItemHeight = 13
    TabOrder = 5
    Items.Strings = (
      'Screen'
      'Raw Text File'
      'Comma Seperated File')
  end
end
