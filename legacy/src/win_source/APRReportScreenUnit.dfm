object APRReportScreen: TAPRReportScreen
  Left = 610
  Top = 180
  Width = 522
  Height = 320
  Caption = 'Truth in lending report of APR'
  Color = clBtnFace
  Font.Charset = DEFAULT_CHARSET
  Font.Color = clWindowText
  Font.Height = -11
  Font.Name = 'MS Sans Serif'
  Font.Style = []
  OldCreateOrder = False
  OnActivate = FormActivate
  OnClose = FormClose
  PixelsPerInch = 96
  TextHeight = 13
  object Label9: TLabel
    Left = 8
    Top = 176
    Width = 138
    Height = 13
    Caption = 'Number of regular payments: '
  end
  object Label10: TLabel
    Left = 8
    Top = 200
    Width = 134
    Height = 13
    Caption = 'Amount of regular payments:'
  end
  object Label11: TLabel
    Left = 8
    Top = 224
    Width = 122
    Height = 13
    Caption = 'When payments are due: '
  end
  object PaymentCountLabel: TLabel
    Left = 160
    Top = 176
    Width = 95
    Height = 13
    Caption = 'PaymentCountLabel'
  end
  object PaymentAmountLabel: TLabel
    Left = 160
    Top = 200
    Width = 103
    Height = 13
    Caption = 'PaymentAmountLabel'
  end
  object PaymentDateLabel: TLabel
    Left = 160
    Top = 224
    Width = 90
    Height = 13
    Caption = 'PaymentDateLabel'
  end
  object ExtraLabel1: TLabel
    Left = 8
    Top = 248
    Width = 56
    Height = 13
    Caption = 'ExtraLabel1'
  end
  object ExtraLabel2: TLabel
    Left = 8
    Top = 272
    Width = 56
    Height = 13
    Caption = 'ExtraLabel2'
  end
  object Panel1: TPanel
    Left = 0
    Top = 0
    Width = 128
    Height = 169
    TabOrder = 0
    object Label1: TLabel
      Left = 16
      Top = 8
      Width = 86
      Height = 60
      Alignment = taCenter
      Caption = 'Annual Percentage Rate'
      Font.Charset = DEFAULT_CHARSET
      Font.Color = clWindowText
      Font.Height = -16
      Font.Name = 'MS Sans Serif'
      Font.Style = []
      ParentFont = False
      WordWrap = True
    end
    object Label5: TLabel
      Left = 8
      Top = 80
      Width = 100
      Height = 26
      Caption = 'The cost of your credit as a yearly rate'
      WordWrap = True
    end
    object APRLabel: TLabel
      Left = 8
      Top = 144
      Width = 48
      Height = 13
      Caption = 'APRLabel'
    end
  end
  object Panel2: TPanel
    Left = 128
    Top = 0
    Width = 128
    Height = 169
    TabOrder = 1
    object Label2: TLabel
      Left = 16
      Top = 8
      Width = 61
      Height = 40
      Alignment = taCenter
      Caption = 'Finance Charge'
      Font.Charset = DEFAULT_CHARSET
      Font.Color = clWindowText
      Font.Height = -16
      Font.Name = 'MS Sans Serif'
      Font.Style = []
      ParentFont = False
      WordWrap = True
    end
    object Label6: TLabel
      Left = 8
      Top = 80
      Width = 103
      Height = 26
      Caption = 'The dollar amount the credit will cost you'
      WordWrap = True
    end
    object FinanceLabel: TLabel
      Left = 8
      Top = 144
      Width = 64
      Height = 13
      Caption = 'FinanceLabel'
    end
  end
  object Panel3: TPanel
    Left = 256
    Top = 0
    Width = 128
    Height = 169
    TabOrder = 2
    object Label3: TLabel
      Left = 16
      Top = 8
      Width = 66
      Height = 40
      Alignment = taCenter
      Caption = 'Amount Financed'
      Font.Charset = DEFAULT_CHARSET
      Font.Color = clWindowText
      Font.Height = -16
      Font.Name = 'MS Sans Serif'
      Font.Style = []
      ParentFont = False
      WordWrap = True
    end
    object Label7: TLabel
      Left = 8
      Top = 80
      Width = 103
      Height = 39
      Caption = 'The amount of credit provided to you or on your behalf'
      WordWrap = True
    end
    object AmountLabel: TLabel
      Left = 8
      Top = 144
      Width = 62
      Height = 13
      Caption = 'AmountLabel'
    end
  end
  object Panel4: TPanel
    Left = 384
    Top = 0
    Width = 129
    Height = 169
    TabOrder = 3
    object Label4: TLabel
      Left = 16
      Top = 8
      Width = 70
      Height = 40
      Alignment = taCenter
      Caption = 'Total Of Payments'
      Font.Charset = DEFAULT_CHARSET
      Font.Color = clWindowText
      Font.Height = -16
      Font.Name = 'MS Sans Serif'
      Font.Style = []
      ParentFont = False
      WordWrap = True
    end
    object Label8: TLabel
      Left = 8
      Top = 80
      Width = 111
      Height = 52
      Caption = 
        'The amount you will have paid after you have made all payments a' +
        's scheduled'
      WordWrap = True
    end
    object TotalLabel: TLabel
      Left = 8
      Top = 144
      Width = 50
      Height = 13
      Caption = 'TotalLabel'
    end
  end
end
