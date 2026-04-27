object AmortizationScreen: TAmortizationScreen
  Left = 474
  Top = 206
  Width = 600
  Height = 420
  Caption = 'Amortization'
  Color = clBtnFace
  Constraints.MinHeight = 400
  Constraints.MinWidth = 600
  Font.Charset = DEFAULT_CHARSET
  Font.Color = clWindowText
  Font.Height = -11
  Font.Name = 'MS Sans Serif'
  Font.Style = []
  FormStyle = fsMDIChild
  OldCreateOrder = False
  Position = poDefault
  Visible = True
  OnClose = FormClose
  OnResize = FormResize
  PixelsPerInch = 96
  TextHeight = 13
  object AdvancedGroup: TGroupBox
    Left = 0
    Top = 72
    Width = 585
    Height = 281
    Ctl3D = True
    ParentCtl3D = False
    TabOrder = 2
    object Label1: TLabel
      Left = 48
      Top = 80
      Width = 84
      Height = 13
      Caption = 'Balloon Payments'
    end
    object Label2: TLabel
      Left = 48
      Top = 96
      Width = 23
      Height = 13
      Caption = 'Date'
    end
    object Label3: TLabel
      Left = 136
      Top = 96
      Width = 36
      Height = 13
      Caption = 'Amount'
    end
    object Label4: TLabel
      Left = 8
      Top = 16
      Width = 139
      Height = 13
      Caption = 'Additional Periodic Payments:'
    end
    object Label5: TLabel
      Left = 16
      Top = 32
      Width = 150
      Height = 13
      Caption = '(0 for regular skipped payments)'
    end
    object Label6: TLabel
      Left = 360
      Top = 80
      Width = 189
      Height = 13
      Caption = 'Rate Changes and Changes in Payment'
    end
    object Label7: TLabel
      Left = 368
      Top = 96
      Width = 23
      Height = 13
      Caption = 'Date'
    end
    object Label8: TLabel
      Left = 456
      Top = 96
      Width = 23
      Height = 13
      Caption = 'Rate'
    end
    object Label9: TLabel
      Left = 512
      Top = 96
      Width = 36
      Height = 13
      Caption = 'Amount'
    end
    object Label10: TLabel
      Left = 40
      Top = 208
      Width = 47
      Height = 26
      Alignment = taCenter
      Caption = 'Int only til Date'
      WordWrap = True
    end
    object Label11: TLabel
      Left = 152
      Top = 208
      Width = 54
      Height = 26
      Alignment = taCenter
      Caption = 'Princ target Amount'
      WordWrap = True
    end
    object Label12: TLabel
      Left = 240
      Top = 208
      Width = 97
      Height = 26
      Alignment = taCenter
      Caption = 'No payment months Skip months'
      WordWrap = True
    end
    object BalloonGrid: TPersenseGrid
      Left = 32
      Top = 112
      Width = 169
      Height = 89
      ColCount = 2
      FixedCols = 0
      RowCount = 4
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssVertical
      TabOrder = 0
      OnSelectCell = BalloonGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = BalloonGridCellAfterEdit
      OnEditEnterKeyPressed = BalloonGridEditEnterKeyPressed
      OnEditCut = BalloonGridEditCut
      OnEditCopy = BalloonGridEditCopy
      OnEditPaste = BalloonGridEditPaste
      OnDownAfterGrid = BalloonGridDownAfterGrid
      OnUpBeforeGrid = BalloonGridUpBeforeGrid
      OnRightAfterGrid = BalloonGridRightAfterGrid
      ColWidths = (
        84
        54)
    end
    object PrepaymentGrid: TPersenseGrid
      Left = 216
      Top = -1
      Width = 209
      Height = 54
      FixedCols = 0
      RowCount = 2
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 1
      OnSelectCell = PrepaymentGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = PrepaymentGridCellAfterEdit
      OnEditEnterKeyPressed = PrepaymentGridEditEnterKeyPressed
      OnEditCut = PrepaymentGridEditCut
      OnEditCopy = PrepaymentGridEditCopy
      OnEditPaste = PrepaymentGridEditPaste
      OnDownAfterGrid = PrepaymentGridDownAfterGrid
      OnUpBeforeGrid = PrepaymentGridUpBeforeGrid
      OnRightAfterGrid = PrepaymentGridRightAfterGrid
      OnLeftBeforeGrid = PrepaymentGridLeftBeforeGrid
      OnVerifyCellString = PrepaymentGridVerifyCellString
      ColWidths = (
        41
        41
        41
        41
        31)
    end
    object AdjustmentGrid: TPersenseGrid
      Left = 344
      Top = 112
      Width = 225
      Height = 89
      ColCount = 3
      FixedCols = 0
      RowCount = 4
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssVertical
      TabOrder = 2
      OnSelectCell = AdjustmentGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = AdjustmentGridCellAfterEdit
      OnEditEnterKeyPressed = AdjustmentGridEditEnterKeyPressed
      OnEditCut = AdjustmentGridEditCut
      OnEditCopy = AdjustmentGridEditCopy
      OnEditPaste = AdjustmentGridEditPaste
      OnDownAfterGrid = AdjustmentGridDownAfterGrid
      OnUpBeforeGrid = AdjustmentGridUpBeforeGrid
      OnLeftBeforeGrid = AdjustmentGridLeftBeforeGrid
      OnVerifyCellString = AdjustmentGridVerifyCellString
      ColWidths = (
        74
        74
        45)
    end
    object MoratoriumGrid: TPersenseGrid
      Left = 32
      Top = 240
      Width = 81
      Height = 25
      ColCount = 1
      FixedCols = 0
      RowCount = 1
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 3
      OnSelectCell = MoratoriumGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = MoratoriumGridCellAfterEdit
      OnEditEnterKeyPressed = MoratoriumGridEditEnterKeyPressed
      OnEditCut = MoratoriumGridEditCut
      OnEditCopy = MoratoriumGridEditCopy
      OnEditPaste = MoratoriumGridEditPaste
      OnDownAfterGrid = MoratoriumGridDownAfterGrid
      OnUpBeforeGrid = MoratoriumGridUpBeforeGrid
      OnRightAfterGrid = MoratoriumGridRightAfterGrid
      OnLeftBeforeGrid = MoratoriumGridLeftBeforeGrid
      ColWidths = (
        71)
    end
    object TargetGrid: TPersenseGrid
      Left = 136
      Top = 240
      Width = 89
      Height = 25
      ColCount = 1
      FixedCols = 0
      RowCount = 1
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 4
      OnSelectCell = TargetGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = TargetGridCellAfterEdit
      OnEditEnterKeyPressed = TargetGridEditEnterKeyPressed
      OnEditCut = TargetGridEditCut
      OnEditCopy = TargetGridEditCopy
      OnEditPaste = TargetGridEditPaste
      OnDownAfterGrid = TargetGridDownAfterGrid
      OnUpBeforeGrid = TargetGridUpBeforeGrid
      OnRightAfterGrid = TargetGridRightAfterGrid
      OnLeftBeforeGrid = TargetGridLeftBeforeGrid
      ColWidths = (
        79)
    end
    object SkipGrid: TPersenseGrid
      Left = 248
      Top = 240
      Width = 89
      Height = 25
      ColCount = 1
      FixedCols = 0
      RowCount = 1
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 5
      OnSelectCell = SkipGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = SkipGridCellAfterEdit
      OnEditEnterKeyPressed = SkipGridEditEnterKeyPressed
      OnEditCut = SkipGridEditCut
      OnEditCopy = SkipGridEditCopy
      OnEditPaste = SkipGridEditPaste
      OnRightAfterGrid = SkipGridRightAfterGrid
      OnLeftBeforeGrid = SkipGridLeftBeforeGrid
      ColWidths = (
        79)
    end
  end
  object AmortGrid: TPersenseGrid
    Left = 0
    Top = 48
    Width = 585
    Height = 28
    ColCount = 10
    FixedCols = 0
    RowCount = 1
    FixedRows = 0
    Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
    ScrollBars = ssNone
    TabOrder = 0
    OnSelectCell = AmortGridSelectCell
    CellFont.Charset = DEFAULT_CHARSET
    CellFont.Color = clWindowText
    CellFont.Height = -11
    CellFont.Name = 'MS Sans Serif'
    CellFont.Style = []
    CellBackgroundColor = clWhite
    OutpCellFont.Charset = DEFAULT_CHARSET
    OutpCellFont.Color = clWindowText
    OutpCellFont.Height = -11
    OutpCellFont.Name = 'MS Sans Serif'
    OutpCellFont.Style = []
    OutpCellBackgroundColor = clMoneyGreen
    SelectedCellColor = clYellow
    IsExpandable = False
    OnCellBeforeEdit = AmortGridCellBeforeEdit
    OnCellAfterEdit = AmortGridCellAfterEdit
    OnEditEnterKeyPressed = AmortGridEditEnterKeyPressed
    OnEditCut = AmortGridEditCut
    OnEditCopy = AmortGridEditCopy
    OnEditPaste = AmortGridEditPaste
    OnDownAfterGrid = AmortGridDownAfterGrid
    OnUpBeforeGrid = AmortGridUpBeforeGrid
    OnVerifyCellString = AmortGridVerifyCellString
    ColWidths = (
      58
      58
      58
      58
      58
      58
      58
      58
      58
      48)
  end
  object GroupBox1: TGroupBox
    Left = 0
    Top = 0
    Width = 592
    Height = 41
    Align = alTop
    TabOrder = 1
    object AmountLabel: TLabel
      Left = 16
      Top = 8
      Width = 45
      Height = 26
      Alignment = taCenter
      Caption = 'Amount Borrowed'
      WordWrap = True
    end
    object LoanDateLabel: TLabel
      Left = 88
      Top = 8
      Width = 27
      Height = 26
      Alignment = taCenter
      Caption = 'Loan Date'
      WordWrap = True
    end
    object LoanRateLabel: TLabel
      Left = 144
      Top = 8
      Width = 34
      Height = 26
      Alignment = taCenter
      Caption = 'Loan Rate %'
      WordWrap = True
    end
    object FirstDateLabel: TLabel
      Left = 184
      Top = 8
      Width = 43
      Height = 26
      Alignment = taCenter
      Caption = 'First Pmt Date'
      WordWrap = True
    end
    object NPeriodsLabel: TLabel
      Left = 256
      Top = 8
      Width = 18
      Height = 26
      Caption = '#of Pds'
      WordWrap = True
    end
    object LastDateLabel: TLabel
      Left = 296
      Top = 8
      Width = 41
      Height = 26
      Alignment = taCenter
      Caption = 'Last Pmt Date'
      WordWrap = True
    end
    object PerYrLabel: TLabel
      Left = 352
      Top = 8
      Width = 27
      Height = 26
      Alignment = taCenter
      Caption = 'Pmts /Year'
      WordWrap = True
    end
    object PayAmtLabel: TLabel
      Left = 392
      Top = 24
      Width = 41
      Height = 13
      Caption = 'Payment'
    end
    object PointsLabel: TLabel
      Left = 448
      Top = 24
      Width = 29
      Height = 13
      Caption = 'Points'
    end
    object APRLabel: TLabel
      Left = 488
      Top = 24
      Width = 33
      Height = 13
      Caption = 'APR %'
    end
  end
  object PayoffBox: TGroupBox
    Left = 384
    Top = 280
    Width = 185
    Height = 65
    Caption = 'Payoff balance calculation'
    TabOrder = 3
    object Label13: TLabel
      Left = 32
      Top = 16
      Width = 23
      Height = 13
      Caption = 'Date'
    end
    object Label14: TLabel
      Left = 96
      Top = 16
      Width = 68
      Height = 13
      Caption = 'Balance as of:'
    end
    object PayoffGrid: TPersenseGrid
      Left = 8
      Top = 32
      Width = 169
      Height = 25
      ColCount = 2
      FixedCols = 0
      RowCount = 1
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 0
      OnSelectCell = PayoffGridSelectCell
      CellFont.Charset = DEFAULT_CHARSET
      CellFont.Color = clWindowText
      CellFont.Height = -11
      CellFont.Name = 'MS Sans Serif'
      CellFont.Style = []
      CellBackgroundColor = clWhite
      OutpCellFont.Charset = DEFAULT_CHARSET
      OutpCellFont.Color = clWindowText
      OutpCellFont.Height = -11
      OutpCellFont.Name = 'MS Sans Serif'
      OutpCellFont.Style = []
      OutpCellBackgroundColor = clMoneyGreen
      SelectedCellColor = clYellow
      IsExpandable = False
      OnCellAfterEdit = PayoffGridCellAfterEdit
      OnEditEnterKeyPressed = PayoffGridEditEnterKeyPressed
      OnEditCut = PayoffGridEditCut
      OnEditCopy = PayoffGridEditCopy
      OnEditPaste = PayoffGridEditPaste
      OnUpBeforeGrid = PayoffGridUpBeforeGrid
      OnRightAfterGrid = PayoffGridRightAfterGrid
      OnLeftBeforeGrid = PayoffGridLeftBeforeGrid
      ColWidths = (
        84
        74)
    end
  end
  object SaveDialog1: TSaveDialog
    Left = 272
    Top = 144
  end
end
