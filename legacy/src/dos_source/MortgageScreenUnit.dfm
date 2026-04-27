object MortgageScreen: TMortgageScreen
  Left = 313
  Top = 119
  Width = 655
  Height = 450
  HorzScrollBar.Visible = False
  VertScrollBar.Range = 349
  VertScrollBar.Visible = False
  AutoScroll = False
  Caption = 'Mortgage'
  Color = clBtnFace
  Constraints.MinHeight = 235
  Constraints.MinWidth = 652
  ParentFont = True
  FormStyle = fsMDIChild
  OldCreateOrder = False
  Position = poDefault
  ShowHint = True
  Visible = True
  OnClose = FormClose
  OnResize = FormResize
  PixelsPerInch = 96
  TextHeight = 13
  object MortgageGrid: TPersenseGrid
    Left = 0
    Top = 139
    Width = 644
    Height = 284
    ColCount = 12
    RowCount = 18
    FixedRows = 0
    Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
    ParentShowHint = False
    ScrollBars = ssVertical
    ShowHint = True
    TabOrder = 0
    OnSelectCell = MortgageGridSelectCell
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
    IsExpandable = True
    OnCellBeforeEdit = MortgageGridCellBeforeEdit
    OnCellAfterEdit = MortgageGridCellAfterEdit
    OnEditEnterKeyPressed = MortgageGridEditEnterKeyPressed
    OnEditCut = MortgageGridEditCut
    OnEditCopy = MortgageGridEditCopy
    OnEditPaste = MortgageGridEditPaste
    OnVerifyCellString = MortgageGridVerifyCellString
    ColWidths = (
      53
      53
      53
      53
      53
      53
      53
      53
      53
      53
      53
      23)
    RowHeights = (
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24
      24)
  end
  object HeaderBox: TGroupBox
    Left = 0
    Top = 24
    Width = 513
    Height = 41
    TabOrder = 1
    object PriceLabel: TLabel
      Left = 9
      Top = 21
      Width = 24
      Height = 13
      Caption = 'Price'
    end
    object PointsLabel: TLabel
      Left = 48
      Top = 21
      Width = 29
      Height = 13
      Caption = 'Points'
    end
    object PercentLabel: TLabel
      Left = 96
      Top = 8
      Width = 28
      Height = 26
      Caption = '   % Down'
      WordWrap = True
    end
    object CashLabel: TLabel
      Left = 152
      Top = 8
      Width = 43
      Height = 26
      Caption = 'Cash Required'
      WordWrap = True
    end
    object AmountLabel: TLabel
      Left = 216
      Top = 8
      Width = 45
      Height = 26
      Caption = 'Amount Borrowed'
      WordWrap = True
    end
    object YearsLabel: TLabel
      Left = 272
      Top = 21
      Width = 27
      Height = 13
      Caption = 'Years'
    end
    object LoanLabel: TLabel
      Left = 328
      Top = 8
      Width = 27
      Height = 26
      Caption = 'Loan Rate'
      WordWrap = True
    end
    object MonthlyLabel: TLabel
      Left = 379
      Top = 8
      Width = 38
      Height = 26
      Caption = 'Monthly Tax+Ins'
      WordWrap = True
    end
    object TotalLabel: TLabel
      Left = 465
      Top = 8
      Width = 40
      Height = 26
      Caption = 'Monthly Total'
      WordWrap = True
    end
  end
  object BalloonBox: TGroupBox
    Left = 425
    Top = 24
    Width = 137
    Height = 41
    Caption = 'Balloon'
    TabOrder = 2
    object BalloonYearsLabel: TLabel
      Left = 14
      Top = 21
      Width = 15
      Height = 13
      Caption = 'Yrs'
    end
    object BalloonAmountLabel: TLabel
      Left = 85
      Top = 21
      Width = 36
      Height = 13
      Caption = 'Amount'
    end
  end
  object SaveDialog1: TSaveDialog
    Left = 24
    Top = 88
  end
end
