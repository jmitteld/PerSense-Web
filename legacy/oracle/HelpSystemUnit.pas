unit HelpSystemUnit;
{ Headless stub for the binary-oracle driver. The real HelpSystemUnit
  is a Win32 HTML-help class (uses Windows/HHCTRL); the computational
  units reference ONLY its message-ID constants, copied verbatim below
  from legacy/src/dos_source/HelpSystemUnit.PAS lines 8-205. }
interface
const
  // the following is a LONG list of all the help codes used in Persense.
  // these codes are assigned in the Help/Help.txt file.  The file has a
  // simple format:
  // $00000000help message
  // where the first 9 chars are the help code and the rest of the text until
  // the <CRLF> is the help message.
  // I assign some special codes to the first byte:
  // 0x00 = Status bar help message
  // 0x01 = Inline example HTML string for help system
  // 0x02 = Dialog or Message box help HTML string for help system
  // 0x04 = Contextual help HTML string for help system
  // second byte specifies which screen
  // 0x00 = Mortgage screen
  // 0x01 = Amortization screen
  // 0x02 = Present Value screen
  // 0x04 = Other screen
  //
  // THE CODE $00000000 IS RESERVED!  DO NOT ASSIGN!
  //
  HELP_NULL                     = $00000000;
  // Status bar help messages
  // mortgage screen status bar messages
  SM_PriceCol                   = $00000001;
  SM_PointsCol                  = $00000002;
  SM_PercentDownCol             = $00000003;
  SM_CashRequiredCol            = $00000004;
  SM_AmountBorrowedCol          = $00000005;
  SM_YearsCol                   = $00000006;
  SM_LoanRateCol                = $00000007;
  SM_MonthlyTaxCol              = $00000008;
  SM_MonthlyTotalCol            = $00000009;
  SM_BalloonYearsCol            = $0000000a;
  SM_BalloonAmountCol           = $0000000b;
  // Amortization screen status bar messages
  SA_AMZAmountCol               = $00010001;
  SA_AMZLoanDateCol             = $00010002;
  SA_AMZLoanRateCol             = $00010003;
  SA_AMZFirstDateCol            = $00010004;
  SA_AMZNPeriodsCol             = $00010005;
  SA_AMZLastDateCol             = $00010006;
  SA_AMZPerYrCol                = $00010007;
  SA_AMZPayAmtCol               = $00010008;
  SA_AMZPointsCol               = $00010009;
  SA_AMZAPRCol                  = $0001000a;
  SA_PREStartCol                = $0001000b;
  SA_PRENNCol                   = $0001000c;
  SA_PREStopCol                 = $0001000d;
  SA_PREPerYrCol                = $0001000e;
  SA_PREPaymentCol              = $0001000f;
  SA_BALDateCol                 = $00010010;
  SA_BALAmountCol               = $00010011;
  SA_ADJDateCol                 = $00010012;
  SA_ADJRateCol                 = $00010013;
  SA_ADJAmountCol               = $00010014;
  SA_Moratorium                 = $00010015;
  SA_Target                     = $00010016;
  SA_Skip                       = $00010017;
  SA_OFFDateCol                 = $00010018;
  SA_OFFAmountCol               = $00010019;
  // Present value status bar text
  SP_LSMDateCol                 = $00020001;
  SP_LSMAmountCol               = $00020002;
  SP_LSMValueCol                = $00020003;
  SP_PERFromDateCol             = $00020004;
  SP_PERToDateCol               = $00020005;
  SP_PERPerYrCol                = $00020006;
  SP_PERAmountCol               = $00020007;
  SP_PERCOLACol                 = $00020008;
  SP_PERValueCol                = $00020009;
  SP_PRVAsOfCol                 = $0002000a;
  SP_PRVTrueRateCol             = $0002000b;
  SP_PRVLoanRateCol             = $0002000c;
  SP_PRVYieldCol                = $0002000d;
  SP_PRVValueCol                = $0002000e;
  SP_RTLDateCol                 = $0002000f;
  SP_RTLTrueRateCol             = $00020011;
  SP_RTLLoanRateCol             = $00020012;
  SP_RTLYieldCol                = $00020013;
  SP_XPRAsOfCol                 = $00020014;
  SP_XPRComputationCol          = $00020015;
  SP_XPRValueCol                = $00020016;
  // inline help HTML strings
  // last 3 bytes is the example number that's being loaded
  // Mortgage HTML strings
  IM_MASK                       = $01000000;
  // Amortization HTML strings
  IA_MASK                       = $01010000;
  // Present Value HTML strings
  IP_MASK                       = $01020000;
  // dialog box help strings
  // mortgage
  DM_YearsNegative              = $02000001;
  DM_SpecifyBalloonPayment      = $02000002;
  DM_AmountBorrowedExceedsPrice = $02000003;
  DM_PriceTooSmall              = $02000004;
  DM_LeaveSomeBlank             = $02000005;
  DM_FillPercentOrCash          = $02000006;
  DM_APRDidNotConverge          = $02000007;
  DM_InsufficientDataIn2nd      = $02000008;
  DM_CrossoverDidNotConverge    = $02000009;
  DM_1OrMore4APR                = $0200000a;
  DA_FillLinesForComparison     = $0200000b;
  DM_NotNEoughDataForAPR        = $0200000c;
  DA_FillRowForComparison       = $0200000d;
  DM_CalcErrorsForGenerate      = $0200000e;
  DM_RowCountExceeded           = $0200000f;
  DM_GeneratedRowError          = $02000010;
  // amortization
  DA_LastPayBeforeFirst         = $02010001;
  DA_ChangeTo365                = $02010002;
  DA_InterestTooSmall           = $02010003;
  DA_ZeroRateLoan               = $02010004;
  DA_DurationIsNegative         = $02010005;
  DA_RateTooSmall               = $02010006;
  DA_TerminatingBalloonChanged  = $02010007;
  DA_LoanBalanceBeforeDate      = $02010008;
  DA_NotEnoughDataForTable      = $02010009;
  DA_2PaymentsMin               = $0201000a;
  DA_DateOutOfOrder             = $0201000b;
  DA_BalloonPrecedesRepay       = $0201000c;
  DA_PrincPrecedesFirstPay      = $0201000d;
  DA_NoAdvanceMsg               = $0201000e;
  DA_PrincPrecedeLastPay        = $0201000f;
  DA_PrincipalReductionTooHigh  = $02010010;
  DA_BorrowedUsingReduction     = $02010011;
  DA_APRNoConverge              = $02010012;
  DA_BalloonPrecedeFirstPay     = $02010013;
  DA_2RateAdjustsPerDay         = $02010014;
  DA_RateChangePrecedeDate      = $02010015;
  DA_RateChangeAfterPay         = $02010016;
  DA_InternalError              = $02010017;
  DA_PaymentTooSmall            = $02010018;
  DA_PayOrInterestNoConverge    = $02010019;
  DA_UnusuallyHighRate          = $0201001a;
  DA_SetBalloonIncludesToNo     = $0201001b;
  DA_SetBalloonIncPmt           = $0201001c;
  DA_OverwriteFile              = $0201001d;
  // present value
  DP_OutOfMemory                = $02020001;
  DP_NoMemoryForTable           = $02020002;
  DP_DateAmountNoValue          = $02020003;
  DP_1Line2Unknowns             = $02020004;
  DP_TooMuchInPaymentBlock      = $02020005;
  DP_PaymentInfinite            = $02020006;
  DP_1Line2UnknownsUpperRight   = $02020007;
  DP_Only2Of3InPayment          = $02020008;
  DP_DatesOutOfOrder            = $02020009;
  DP_RateNotDetermind           = $0202000a;
  DP_RateDidNotConverge         = $0202000b;
  DP_InterestTooSmall           = $0202000c;
  DP_ComputationNoConvergeBy    = $0202000d;
  DP_PositiveNegativeMessage    = $0202000e;
  DP_DateNotConvergeBy          = $0202000f;
  DP_DateOrAmountInSingle       = $02020010;
  DP_DateOrAmountInPeriodic     = $02020011;
  DP_InsufficientDataForTable   = $02020012;
  DP_TooManyUnknowns            = $02020013;
  DP_FromLaterThenThrough       = $02020014;
  DP_1Line2UnknownsUpperBlock   = $02020015;
  DP_1Line2UnknownsTopRight     = $02020016;
  DP_OutOfOrderRateList         = $02020017;
  DP_RedeterminedValue          = $02020018;
  DP_ReDeterminedData           = $02020019;
  DP_OverwriteFile              = $0202001a;
  // other
  DO_OutOfMemory                = $02040001;
  DO_TimeTooLong                = $02040002;
  DO_FindBlockNotFound          = $02040003;
  DO_SqrrtTiny                  = $02040004;
  DO_ExxpOverflow               = $02040005;
  DO_LnnNegative                = $02040006;
  DO_OpeningFile                = $02040007;
  DO_InvalidFile                = $02040008;
  DO_UnkownFileType             = $02040009;
  DO_LoanAmountMustBePositive   = $0204000a;
  DO_InsufficentDataForAPR      = $0204000b;
  DO_APRDidNotConverge          = $0204000c;
  DO_OverwriteFile              = $0204000d;
  DO_UnsavedMortgage            = $0204000e;
  DO_UnsavedAmortization        = $0204000f;
  DO_UnsavedPresentValue        = $02040010;
  // end of help codes

  HH_INITIALIZE                 = $001C;
  HH_UNINITIALIZE               = $001D;
  HH_DISPLAY_TOC                = $0001;
  HH_GET_WIN_TYPE               = $0005;
  HH_CLOSE_ALL                  = $0012;

  HHN_FIRST                     = (0-860);
  HHN_LAST                      = (0-879);
  HHN_NAVCOMPLETE               = (HHN_FIRST-0);
  HHN_TRACK                     = (HHN_FIRST-1);
  HHN_WINDOW_CREATE             = (HHN_FIRST-2);

  NOTIFY_MESSAGE_ID             = $6969;


implementation
end.
