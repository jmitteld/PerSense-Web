object TableOut: TTableOut
  Left = 510
  Top = 154
  Width = 613
  Height = 461
  Caption = 'Table Output'
  Color = clBtnFace
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
    Top = 48
    Width = 605
    Height = 386
    Align = alBottom
    Anchors = [akTop]
    Font.Charset = ANSI_CHARSET
    Font.Color = clWindowText
    Font.Height = -11
    Font.Name = 'Fixedsys'
    Font.Style = []
    ParentFont = False
    ReadOnly = True
    ScrollBars = ssVertical
    TabOrder = 0
  end
  object GroupBox1: TGroupBox
    Left = 0
    Top = 0
    Width = 605
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
