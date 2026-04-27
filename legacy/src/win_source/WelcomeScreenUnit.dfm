object WelcomeScreen: TWelcomeScreen
  Left = 257
  Top = 211
  Width = 455
  Height = 370
  Caption = 'Per%Sense'
  Color = clSkyBlue
  Constraints.MaxHeight = 370
  Constraints.MaxWidth = 455
  Constraints.MinHeight = 370
  Constraints.MinWidth = 455
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
  object Label1: TLabel
    Left = 32
    Top = 16
    Width = 157
    Height = 36
    Caption = 'Per%Sense'
    Color = clSkyBlue
    Font.Charset = ANSI_CHARSET
    Font.Color = clWindowText
    Font.Height = -32
    Font.Name = 'Garamond'
    Font.Style = [fsBold]
    ParentColor = False
    ParentFont = False
    Layout = tlCenter
  end
  object Label2: TLabel
    Left = 240
    Top = 8
    Width = 134
    Height = 48
    Caption = '"Every financial calculation you can imagine"'
    Font.Charset = ANSI_CHARSET
    Font.Color = clWindowText
    Font.Height = -13
    Font.Name = 'MS Serif'
    Font.Style = [fsBold, fsItalic]
    ParentFont = False
    WordWrap = True
  end
  object GroupBox1: TGroupBox
    Left = 0
    Top = 64
    Width = 443
    Height = 249
    TabOrder = 0
    object Label7: TLabel
      Left = 208
      Top = 216
      Width = 135
      Height = 26
      Alignment = taCenter
      Caption = 'A good place to start setting up your calculation. '
      WordWrap = True
    end
    object HelpButton: TButton
      Left = 80
      Top = 184
      Width = 75
      Height = 25
      Action = MainForm.HelpContents1
      Font.Charset = DEFAULT_CHARSET
      Font.Color = clWindowText
      Font.Height = -13
      Font.Name = 'MS Sans Serif'
      Font.Style = []
      ParentFont = False
      TabOrder = 0
    end
    object ExamplesButton: TButton
      Left = 224
      Top = 184
      Width = 83
      Height = 25
      Action = MainForm.HelpExamples1
      Font.Charset = DEFAULT_CHARSET
      Font.Color = clWindowText
      Font.Height = -13
      Font.Name = 'MS Sans Serif'
      Font.Style = []
      ParentFont = False
      TabOrder = 1
    end
    object GroupBox2: TGroupBox
      Left = 8
      Top = 16
      Width = 427
      Height = 155
      Caption = 'The 3 Per%Sense worksheets'
      TabOrder = 2
      object Label3: TLabel
        Left = 182
        Top = 32
        Width = 216
        Height = 26
        Caption = 'Simple monthly loans and smart comparison       for refi.'
        Color = clSkyBlue
        Font.Charset = DEFAULT_CHARSET
        Font.Color = clMaroon
        Font.Height = -11
        Font.Name = 'MS Sans Serif'
        Font.Style = []
        ParentColor = False
        ParentFont = False
        WordWrap = True
      end
      object Label4: TLabel
        Left = 183
        Top = 72
        Width = 232
        Height = 26
        Caption = 
          'Payment calc and tables for simple loans, or structured, with ra' +
          'te changes balloons, and more.'
        Color = clSkyBlue
        Font.Charset = DEFAULT_CHARSET
        Font.Color = clTeal
        Font.Height = -11
        Font.Name = 'MS Sans Serif'
        Font.Style = []
        ParentColor = False
        ParentFont = False
        WordWrap = True
      end
      object Label5: TLabel
        Left = 185
        Top = 116
        Width = 223
        Height = 26
        Caption = 'A flexible tool for IRRs, valuation, and payment structures.'
        Color = clSkyBlue
        Font.Charset = DEFAULT_CHARSET
        Font.Color = clPurple
        Font.Height = -11
        Font.Name = 'MS Sans Serif'
        Font.Style = []
        ParentColor = False
        ParentFont = False
        WordWrap = True
      end
      object MortgageButton: TButton
        Left = 8
        Top = 32
        Width = 153
        Height = 25
        Action = MainForm.ScreenShowMortgage
        Font.Charset = DEFAULT_CHARSET
        Font.Color = clWindowText
        Font.Height = -13
        Font.Name = 'MS Sans Serif'
        Font.Style = [fsBold]
        ParentFont = False
        TabOrder = 0
      end
      object PresentValueButton: TButton
        Left = 8
        Top = 112
        Width = 153
        Height = 25
        Action = MainForm.ScreenShowPresentValue
        Font.Charset = DEFAULT_CHARSET
        Font.Color = clWindowText
        Font.Height = -13
        Font.Name = 'MS Sans Serif'
        Font.Style = [fsBold]
        ParentFont = False
        TabOrder = 1
      end
      object AmortizationButton: TButton
        Left = 8
        Top = 72
        Width = 153
        Height = 25
        Action = MainForm.ScreenShowAmortization
        Font.Charset = DEFAULT_CHARSET
        Font.Color = clWindowText
        Font.Height = -13
        Font.Name = 'MS Sans Serif'
        Font.Style = [fsBold]
        ParentFont = False
        TabOrder = 2
      end
    end
  end
end
