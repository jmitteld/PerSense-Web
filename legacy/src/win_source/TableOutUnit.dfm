object TableOut: TTableOut
  Left = 337
  Top = 142
  Width = 630
  Height = 500
  Caption = 'Table Output'
  Color = clBtnFace
  Constraints.MinHeight = 500
  Constraints.MinWidth = 630
  Font.Charset = DEFAULT_CHARSET
  Font.Color = clWindowText
  Font.Height = -11
  Font.Name = 'MS Sans Serif'
  Font.Style = []
  OldCreateOrder = False
  OnClose = FormClose
  PixelsPerInch = 96
  TextHeight = 13
  object OutputBox: TMemo
    Left = 0
    Top = 41
    Width = 622
    Height = 432
    Align = alClient
    Font.Charset = ANSI_CHARSET
    Font.Color = clWindowText
    Font.Height = -11
    Font.Name = 'Fixedsys'
    Font.Style = []
    ParentFont = False
    ReadOnly = True
    ScrollBars = ssBoth
    TabOrder = 0
    WordWrap = False
  end
  object GroupBox1: TGroupBox
    Left = 0
    Top = 0
    Width = 622
    Height = 41
    Align = alTop
    TabOrder = 1
    object HeaderLine1: TLabel
      Left = 8
      Top = 8
      Width = 88
      Height = 15
      Caption = 'HeaderLine1'
      Font.Charset = ANSI_CHARSET
      Font.Color = clWindowText
      Font.Height = -11
      Font.Name = 'Fixedsys'
      Font.Style = []
      ParentFont = False
    end
    object HeaderLine2: TLabel
      Left = 8
      Top = 24
      Width = 88
      Height = 15
      Caption = 'HeaderLine2'
      Font.Charset = ANSI_CHARSET
      Font.Color = clWindowText
      Font.Height = -11
      Font.Name = 'Fixedsys'
      Font.Style = []
      ParentFont = False
    end
  end
end
