object PresentValueScreen: TPresentValueScreen
  Left = 550
  Top = 322
  Width = 670
  Height = 367
  Caption = 'PresentValueScreen'
  Color = clBtnFace
  Constraints.MinHeight = 354
  Constraints.MinWidth = 670
  Font.Charset = DEFAULT_CHARSET
  Font.Color = clWindowText
  Font.Height = -11
  Font.Name = 'MS Sans Serif'
  Font.Style = []
  OldCreateOrder = False
  OnClose = FormClose
  OnResize = FormResize
  PixelsPerInch = 96
  TextHeight = 13
  object LumpSumGrid: TPersenseGrid
    Left = 0
    Top = 39
    Width = 249
    Height = 90
    ColCount = 3
    FixedCols = 0
    RowCount = 4
    FixedRows = 0
    Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
    ScrollBars = ssVertical
    TabOrder = 0
    OnSelectCell = LumpSumGridSelectCell
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
    OnCellBeforeEdit = LumpSumGridCellBeforeEdit
    OnCellAfterEdit = LumpSumGridCellAfterEdit
    OnEditEnterKeyPressed = LumpSumGridEditEnterKeyPressed
    OnEditCut = LumpSumGridEditCut
    OnEditCopy = LumpSumGridEditCopy
    OnEditPaste = LumpSumGridEditPaste
    OnDownAfterGrid = LumpSumGridDownAfterGrid
    OnRightAfterGrid = LumpSumGridRightAfterGrid
    OnLeftBeforeGrid = LumpSumGridLeftBeforeGrid
    ColWidths = (
      82
      82
      53)
  end
  object PeriodicGrid: TPersenseGrid
    Left = 256
    Top = 39
    Width = 353
    Height = 90
    ColCount = 6
    FixedCols = 0
    RowCount = 4
    FixedRows = 0
    Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
    ScrollBars = ssVertical
    TabOrder = 1
    OnSelectCell = PeriodicGridSelectCell
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
    OnCellBeforeEdit = PeriodicGridCellBeforeEdit
    OnCellAfterEdit = PeriodicGridCellAfterEdit
    OnEditEnterKeyPressed = PeriodicGridEditEnterKeyPressed
    OnEditCut = PeriodicGridEditCut
    OnEditCopy = PeriodicGridEditCopy
    OnEditPaste = PeriodicGridEditPaste
    OnDownAfterGrid = PeriodicGridDownAfterGrid
    OnRightAfterGrid = PeriodicGridRightAfterGrid
    OnLeftBeforeGrid = PeriodicGridLeftBeforeGrid
    OnVerifyCellString = PeriodicGridVerifyCellString
    ColWidths = (
      58
      58
      58
      58
      58
      28)
  end
  object GroupBox1: TGroupBox
    Left = 0
    Top = 0
    Width = 249
    Height = 33
    Caption = 'Single Payments:'
    TabOrder = 2
    object SingleDateLabel: TLabel
      Left = 24
      Top = 16
      Width = 23
      Height = 13
      Caption = 'Date'
    end
    object SingleAmountLabel: TLabel
      Left = 88
      Top = 16
      Width = 36
      Height = 13
      Caption = 'Amount'
    end
    object SingleValueLabel: TLabel
      Left = 176
      Top = 16
      Width = 27
      Height = 13
      Caption = 'Value'
    end
  end
  object GroupBox2: TGroupBox
    Left = 256
    Top = 0
    Width = 353
    Height = 33
    Caption = 'Periodic Payments:'
    TabOrder = 3
    object PeriodicFromLabel: TLabel
      Left = 16
      Top = 16
      Width = 23
      Height = 13
      Caption = 'From'
    end
    object PeriodicThroughLabel: TLabel
      Left = 72
      Top = 16
      Width = 40
      Height = 13
      Caption = 'Through'
    end
    object PeriodicPerYrLabel: TLabel
      Left = 128
      Top = 16
      Width = 26
      Height = 13
      Caption = 'PerYr'
    end
    object PeriodicAmountLabel: TLabel
      Left = 176
      Top = 16
      Width = 36
      Height = 13
      Caption = 'Amount'
    end
    object PeriodicColaLabel: TLabel
      Left = 232
      Top = 16
      Width = 36
      Height = 13
      Caption = 'COLA%'
    end
    object PeriodicValueLabel: TLabel
      Left = 296
      Top = 16
      Width = 27
      Height = 13
      Caption = 'Value'
    end
  end
  object AdvancedGroup: TGroupBox
    Left = 0
    Top = 136
    Width = 609
    Height = 185
    TabOrder = 5
    Visible = False
    object GroupBox4: TGroupBox
      Left = 8
      Top = 16
      Width = 289
      Height = 49
      TabOrder = 0
      object Label6: TLabel
        Left = 8
        Top = 16
        Width = 42
        Height = 26
        Alignment = taCenter
        Caption = 'Effective Date'
        WordWrap = True
      end
      object Label7: TLabel
        Left = 88
        Top = 16
        Width = 34
        Height = 26
        Alignment = taCenter
        Caption = 'True Rate %'
        WordWrap = True
      end
      object Label8: TLabel
        Left = 160
        Top = 16
        Width = 34
        Height = 26
        Alignment = taCenter
        Caption = 'Loan Rate %'
        WordWrap = True
      end
      object Label9: TLabel
        Left = 232
        Top = 24
        Width = 34
        Height = 13
        Alignment = taCenter
        Caption = 'Yield %'
        WordWrap = True
      end
    end
    object RatelineGrid: TPersenseGrid
      Left = 8
      Top = 71
      Width = 289
      Height = 106
      ColCount = 4
      FixedCols = 0
      RowCount = 3
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssVertical
      TabOrder = 1
      OnSelectCell = RatelineGridSelectCell
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
      OnCellAfterEdit = RatelineGridCellAfterEdit
      OnEditEnterKeyPressed = RatelineGridEditEnterKeyPressed
      OnEditCut = RatelineGridEditCut
      OnEditCopy = RatelineGridEditCopy
      OnEditPaste = RatelineGridEditPaste
      OnUpBeforeGrid = RatelineGridUpBeforeGrid
      OnRightAfterGrid = RatelineGridRightAfterGrid
      OnLeftBeforeGrid = RatelineGridLeftBeforeGrid
      OnVerifyCellString = RatelineGridVerifyCellString
      ColWidths = (
        72
        72
        72
        62)
    end
    object GroupBox5: TGroupBox
      Left = 328
      Top = 16
      Width = 273
      Height = 73
      TabOrder = 2
      object Label10: TLabel
        Left = 56
        Top = 16
        Width = 178
        Height = 13
        Caption = 'Value computed with rate table at left:'
      end
      object Label11: TLabel
        Left = 32
        Top = 48
        Width = 24
        Height = 13
        Caption = 'As of'
      end
      object Label12: TLabel
        Left = 96
        Top = 40
        Width = 59
        Height = 26
        Alignment = taCenter
        Caption = 'Interest Computation'
        WordWrap = True
      end
      object Label13: TLabel
        Left = 208
        Top = 48
        Width = 54
        Height = 13
        Caption = 'Total Value'
      end
    end
    object XPresValGrid: TPersenseGrid
      Left = 328
      Top = 96
      Width = 273
      Height = 28
      ColCount = 3
      FixedCols = 0
      RowCount = 1
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 3
      OnSelectCell = XPresValGridSelectCell
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
      OnCellAfterEdit = XPresValGridCellAfterEdit
      OnEditEnterKeyPressed = XPresValGridEditEnterKeyPressed
      OnEditCut = XPresValGridEditCut
      OnEditCopy = XPresValGridEditCopy
      OnEditPaste = XPresValGridEditPaste
      OnUpBeforeGrid = XPresValGridUpBeforeGrid
      OnLeftBeforeGrid = XPresValGridLeftBeforeGrid
      ColWidths = (
        90
        90
        81)
    end
  end
  object PlainGroup: TGroupBox
    Left = 120
    Top = 152
    Width = 401
    Height = 161
    Ctl3D = True
    ParentCtl3D = False
    TabOrder = 4
    object GroupBox3: TGroupBox
      Left = 8
      Top = 16
      Width = 329
      Height = 49
      Caption = 'Present Value:'
      TabOrder = 0
      object Label1: TLabel
        Left = 24
        Top = 32
        Width = 26
        Height = 13
        Caption = 'As Of'
      end
      object Label2: TLabel
        Left = 72
        Top = 16
        Width = 34
        Height = 26
        Alignment = taCenter
        Caption = 'True Rate %'
        WordWrap = True
      end
      object Label3: TLabel
        Left = 136
        Top = 16
        Width = 34
        Height = 26
        Alignment = taCenter
        Caption = 'Loan Rate %'
        WordWrap = True
      end
      object Label4: TLabel
        Left = 216
        Top = 32
        Width = 34
        Height = 13
        Caption = 'Yield %'
      end
      object Label5: TLabel
        Left = 280
        Top = 32
        Width = 27
        Height = 13
        Caption = 'Value'
      end
    end
    object PresentValueGrid: TPersenseGrid
      Left = 8
      Top = 71
      Width = 329
      Height = 79
      FixedCols = 0
      RowCount = 3
      FixedRows = 0
      Options = [goFixedVertLine, goFixedHorzLine, goVertLine, goHorzLine, goEditing, goTabs]
      ScrollBars = ssNone
      TabOrder = 1
      OnSelectCell = PresentValueGridSelectCell
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
      OnCellAfterEdit = PresentValueGridCellAfterEdit
      OnEditEnterKeyPressed = PresentValueGridEditEnterKeyPressed
      OnEditCut = PresentValueGridEditCut
      OnEditCopy = PresentValueGridEditCopy
      OnEditPaste = PresentValueGridEditPaste
      OnUpBeforeGrid = PresentValueGridUpBeforeGrid
      OnVerifyCellString = PresentValueGridVerifyCellString
      ColWidths = (
        65
        65
        65
        65
        55)
    end
  end
  object SaveDialog1: TSaveDialog
    Left = 520
    Top = 264
  end
end
