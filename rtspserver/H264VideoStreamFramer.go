package rtspserver

var SPS_MAX_SIZE uint = 1000
var SEI_MAX_SIZE uint = 5000 // larger than the largest possible SEI NAL unit

var NUM_NEXT_SLICE_HEADER_BYTES_TO_ANALYZE uint = 12

//////// H264VideoStreamParser ////////
type H264VideoStreamParser struct {
	MPEGVideoStreamParser
	outputStartCodeSize        int
	haveSeenFirstStartCode     bool
	haveSeenFirstByteOfNALUnit bool
	firstByteOfNALUnit         uint
}

func NewH264VideoStreamParser() *H264VideoStreamParser {
	return new(H264VideoStreamParser)
}

func (this *H264VideoStreamParser) UsingSource() *H264VideoStreamFramer {
	return this.usingSource
}

func (this *H264VideoStreamParser) parse() uint {
	if !this.haveSeenFirstStartCode {
		for first4Bytes := this.test4Bytes(); first4Bytes != 0x00000001; {
			this.get1Byte()
			this.setParseState()
			//fmt.Println("parse", first4Bytes)
		}

		this.skipBytes(4)
		this.haveSeenFirstStartCode = true
	}

	if this.outputStartCodeSize > 0 && this.curFrameSize() == 0 && !this.HaveSeenEOF() {
		// Include a start code in the output:
		this.save4Bytes(0x00000001)
	}

	if this.HaveSeenEOF() {
		// We hit EOF the last time that we tried to parse this data, so we know that any remaining unparsed data
		// forms a complete NAL unit, and that there's no 'start code' at the end:
		remainingDataSize := this.TotNumValidBytes() //- this.curOffset()
		for remainingDataSize > 0 {
			nextByte := this.get1Byte()
			if !this.haveSeenFirstByteOfNALUnit {
				this.firstByteOfNALUnit = nextByte
				this.haveSeenFirstByteOfNALUnit = true
			}
			this.saveByte(nextByte)
			remainingDataSize--
		}

		this.get1Byte() // forces another read, which will cause EOF to get handled for real this time
		return 0
	} else {
		next4Bytes := this.test4Bytes()
		if !this.haveSeenFirstByteOfNALUnit {
			this.firstByteOfNALUnit = next4Bytes >> 24
			this.haveSeenFirstByteOfNALUnit = true
		}
		for next4Bytes != 0x00000001 && (next4Bytes&0xFFFFFF00) != 0x00000100 {
			// We save at least some of "next4Bytes".
			if next4Bytes&0xFF > 1 {
				// Common case: 0x00000001 or 0x000001 definitely doesn't begin anywhere in "next4Bytes", so we save all of it:
				this.save4Bytes(next4Bytes)
				this.skipBytes(4)
			} else {
				// Save the first byte, and continue testing the rest:
				this.saveByte(next4Bytes >> 24)
				this.skipBytes(1)
			}
			this.setParseState() // ensures forward progress
			next4Bytes = this.test4Bytes()
		}
		// Assert: next4Bytes starts with 0x00000001 or 0x000001, and we've saved all previous bytes (forming a complete NAL unit).
		// Skip over these remaining bytes, up until the start of the next NAL unit:
		if next4Bytes == 0x00000001 {
			this.skipBytes(4)
		} else {
			this.skipBytes(3)
		}
	}

	nal_ref_idc := this.firstByteOfNALUnit & 0x60 >> 5
	nal_unit_type := this.firstByteOfNALUnit & 0x1F
	this.haveSeenFirstByteOfNALUnit = false // for the next NAL unit that we parse

	switch nal_unit_type {
	case 6: // Supplemental enhancement information (SEI)
		// Later, perhaps adjust "fPresentationTime" if we saw a "pic_timing" SEI payload??? #####
		this.analyzeSEIData()
	case 7: // Sequence parameter set
		// First, save a copy of this NAL unit, in case the downstream object wants to see it:
		//this.UsingSource().saveCopyOfSPS(this.startOfFrame+this.outputStartCodeSize, this.buffTo-this.startOfFrame-this.outputStartCodeSize)

		// Parse this NAL unit to check whether frame rate information is present:
		//num_units_in_tick, time_scale, fixed_frame_rate_flag
		//analyze_seq_parameter_set_data(num_units_in_tick, time_scale, fixed_frame_rate_flag)
		//if time_scale > 0 && num_units_in_tick > 0 {
		//	this.UsingSource().frameRate = time_scale / (2.0 * num_units_in_tick)
		//} else {
		//}
	case 8: // Picture parameter set
		// Save a copy of this NAL unit, in case the downstream object wants to see it:
		//this.UsingSource().saveCopyOfPPS(this.startOfFrame+this.outputStartCodeSize, this.buffTo-this.startOfFrame-this.outputStartCodeSize)
	}

	this.UsingSource().setPresentationTime()

	thisNALUnitEndsAccessUnit := false // until we learn otherwise
	if this.HaveSeenEOF() {
		// There is no next NAL unit, so we assume that this one ends the current 'access unit':
		thisNALUnitEndsAccessUnit = true
	} else {
		isVCL := nal_unit_type <= 5 && nal_unit_type > 0 // Would need to include type 20 for SVC and MVC #####
		if isVCL {
			var firstByteOfNextNALUnit uint
			//this.testBytes(firstByteOfNextNALUnit, 1)
			next_nal_ref_idc := (firstByteOfNextNALUnit & 0x60) >> 5
			next_nal_unit_type := firstByteOfNextNALUnit & 0x1F
			if next_nal_unit_type >= 6 {
				// The next NAL unit is not a VCL; therefore, we assume that this NAL unit ends the current 'access unit':
				thisNALUnitEndsAccessUnit = true
			} else {
				// The next NAL unit is also a VCL.  We need to examine it a little to figure out if it's a different 'access unit'.
				// (We use many of the criteria described in section 7.4.1.2.4 of the H.264 specification.)
				var IdrPicFlag bool
				if nal_unit_type == 5 {
					IdrPicFlag = true
				}
				var next_IdrPicFlag bool
				if next_nal_unit_type == 5 {
					next_IdrPicFlag = true
				}

				if next_IdrPicFlag != IdrPicFlag {
					// IdrPicFlag differs in value
					thisNALUnitEndsAccessUnit = true
				} else if next_nal_ref_idc != nal_ref_idc && next_nal_ref_idc*nal_ref_idc == 0 {
					// nal_ref_idc differs in value with one of the nal_ref_idc values being equal to 0
					thisNALUnitEndsAccessUnit = true
				} else if (nal_unit_type == 1 ||
					nal_unit_type == 2 ||
					nal_unit_type == 5) && (next_nal_unit_type == 1 ||
					next_nal_unit_type == 2 ||
					next_nal_unit_type == 5) {
					// Both this and the next NAL units begin with a "slice_header".
					// Parse this (for each), to get parameters that we can compare:

					// Current NAL unit's "slice_header":
					//this.analyzeSliceHeader(this.startOfFrame+this.outputStartCodeSize, this.buffTo, nal_unit_type, frame_num, pic_parameter_set_id, idr_pic_id, field_pic_flag, bottom_field_flag)

					// Next NAL unit's "slice_header":
					next_slice_header := make([]byte, NUM_NEXT_SLICE_HEADER_BYTES_TO_ANALYZE)
					this.testBytes(next_slice_header, NUM_NEXT_SLICE_HEADER_BYTES_TO_ANALYZE)

					var next_frame_num, next_pic_parameter_set_id, next_idr_pic_id, frame_num, pic_parameter_set_id uint
					var next_field_pic_flag bool
					//this.analyzeSliceHeader(next_slice_header, &next_slice_header[:next_slice_header], next_nal_unit_type, next_frame_num, next_pic_parameter_set_id, next_idr_pic_id, next_field_pic_flag, next_bottom_field_flag)

					var next_bottom_field_flag, bottom_field_flag, idr_pic_id uint
					var field_pic_flag bool
					if next_frame_num != frame_num {
						// frame_num differs in value
						thisNALUnitEndsAccessUnit = true
					} else if next_pic_parameter_set_id != pic_parameter_set_id {
						// pic_parameter_set_id differs in value
						thisNALUnitEndsAccessUnit = true
					} else if next_field_pic_flag != field_pic_flag {
						// field_pic_flag differs in value
						thisNALUnitEndsAccessUnit = true
					} else if next_bottom_field_flag != bottom_field_flag {
						// bottom_field_flag differs in value
						thisNALUnitEndsAccessUnit = true
					} else if next_IdrPicFlag == true && next_idr_pic_id != idr_pic_id {
						// IdrPicFlag is equal to 1 for both and idr_pic_id differs in value
						// Note: We already know that IdrPicFlag is the same for both.
						thisNALUnitEndsAccessUnit = true
					}
				}
			}
		}
	}

	if thisNALUnitEndsAccessUnit {
		this.UsingSource().pictureEndMarker = true
		this.UsingSource().pictureCount++

		// Note that the presentation time for the next NAL unit will be different:
		nextPT := this.UsingSource().nextPresentationTime // alias
		nextPT = this.UsingSource().presentationTime
		nextFraction := nextPT.Tv_usec/1000000.0 + 1/int64(this.UsingSource().frameRate)
		nextSecsIncrement := nextFraction
		nextPT.Tv_sec += nextSecsIncrement
		nextPT.Tv_usec = (nextFraction - nextSecsIncrement) * 1000000
	}
	this.setParseState()

	return this.curFrameSize()
}

func (this *H264VideoStreamParser) removeEmulationBytes(nalUnitCopy []byte, maxSize uint) uint {
	nalUnitOrig := this.startOfFrame //+ this.outputStartCodeSize
	var NumBytesInNALunit uint       //this.buffTo - nalUnitOrig
	if NumBytesInNALunit > maxSize {
		return 0
	}
	var nalUnitCopySize, i uint
	for i = 0; i < NumBytesInNALunit; i++ {
		if i+2 < NumBytesInNALunit && nalUnitOrig[i] == 0 && nalUnitOrig[i+1] == 0 && nalUnitOrig[i+2] == 3 {
			nalUnitCopy[nalUnitCopySize] = nalUnitOrig[i]
			i++
			nalUnitCopySize++
			nalUnitCopy[nalUnitCopySize] = nalUnitOrig[i]
			i++
			nalUnitCopySize++
		} else {
			nalUnitCopy[nalUnitCopySize] = nalUnitOrig[i]
			nalUnitCopySize++
		}
	}
	return nalUnitCopySize
}

func (this *H264VideoStreamParser) analyzeSliceHeader() {
	//var start, end uint
	bv := new(BitVector) //NewBitVector(start, 0, 8*(end-start))

	// Some of the result parameters might not be present in the header; set them to default values:
	//var field_pic_flag, bottom_field_flag bool

	// Note: We assume that there aren't any 'emulation prevention' bytes here to worry about...
	bv.skipBits(8) // forbidden_zero_bit; nal_ref_idc; nal_unit_type
	//first_mb_in_slice := bv.get_expGolomb()
	//slice_type := bv.get_expGolomb()
	//pic_parameter_set_id := bv.get_expGolomb()
	//var separate_colour_plane_flag bool
	//if separate_colour_plane_flag {
	//	bv.skipBits(2) // colour_plane_id
	//}

	//var log2_max_frame_num, nal_unit_type uint
	//var frame_mbs_only_flag bool
	//frame_num := bv.getBits(log2_max_frame_num)
	//if !frame_mbs_only_flag {
	//    field_pic_flag := bv.get1BitBoolean()
	//	if field_pic_flag {
	//        bottom_field_flag := bv.get1BitBoolean()
	//	}
	//}

	//var IdrPicFlag bool
	//if nal_unit_type == 5 {
	//    IdrPicFlag = true
	//}

	//if IdrPicFlag {
	//	idr_pic_id = bv.get_expGolomb()
	//}
}

func (this *H264VideoStreamParser) analyzeSPSData() {
	//time_scale := 0
	//num_units_in_tick := 0
	//fixed_frame_rate_flag := 0 // default values

	// Begin by making a copy of the NAL unit data, removing any 'emulation prevention' bytes:
	sps := make([]byte, SPS_MAX_SIZE)
	spsSize := this.removeEmulationBytes(sps, SPS_MAX_SIZE)

	bv := NewBitVector(sps, 0, 8*spsSize)

	bv.skipBits(8) // forbidden_zero_bit; nal_ref_idc; nal_unit_type
	profile_idc := bv.getBits(8)
	//constraint_setN_flag := bv.getBits(8) // also "reserved_zero_2bits" at end
	//level_idc := bv.getBits(8)
	//seq_parameter_set_id := bv.get_expGolomb()
	if profile_idc == 100 || profile_idc == 110 || profile_idc == 122 || profile_idc == 244 || profile_idc == 44 || profile_idc == 83 || profile_idc == 86 || profile_idc == 118 || profile_idc == 128 {
		chroma_format_idc := bv.get_expGolomb()
		if chroma_format_idc == 3 {
			//eparate_colour_plane_flag := bv.get1BitBoolean()
		}
	}

	bv.get_expGolomb() // bit_depth_luma_minus8
	bv.get_expGolomb() // bit_depth_chroma_minus8
	bv.skipBits(1)     // qpprime_y_zero_transform_bypass_flag
	seq_scaling_matrix_present_flag := bv.get1Bit()
	if seq_scaling_matrix_present_flag != 0 {
		cond := 12
		var chroma_format_idc uint
		if chroma_format_idc != 3 {
			//cond := 8
		}

		for i := 0; i < cond; i++ {
			seq_scaling_list_present_flag := bv.get1Bit()
			if seq_scaling_list_present_flag != 0 {
				sizeOfScalingList := 24
				if i < 6 {
					sizeOfScalingList = 16
				}
				var lastScale uint = 8
				var nextScale uint = 8
				for j := 0; j < sizeOfScalingList; j++ {
					if nextScale != 0 {
						delta_scale := bv.get_expGolomb()
						nextScale = (lastScale + delta_scale + 256) % 256
					}
					if nextScale != 0 {
						lastScale = nextScale
					}
				}
			}
		}
	}

	//log2_max_frame_num_minus4 := bv.get_expGolomb()
	//log2_max_frame_num := log2_max_frame_num_minus4 + 4
	pic_order_cnt_type := bv.get_expGolomb()
	if pic_order_cnt_type == 0 {
		//log2_max_pic_order_cnt_lsb_minus4 := bv.get_expGolomb()
	} else if pic_order_cnt_type == 1 {
		bv.skipBits(1)     // delta_pic_order_always_zero_flag
		bv.get_expGolomb() // offset_for_non_ref_pic
		bv.get_expGolomb() // offset_for_top_to_bottom_field
		num_ref_frames_in_pic_order_cnt_cycle := bv.get_expGolomb()
		var i uint
		for i = 0; i < num_ref_frames_in_pic_order_cnt_cycle; i++ {
			bv.get_expGolomb() // offset_for_ref_frame[i]
		}
	}
	//max_num_ref_frames := bv.get_expGolomb()
	//gaps_in_frame_num_value_allowed_flag := bv.get1Bit()
	//pic_width_in_mbs_minus1 := bv.get_expGolomb()
	//pic_height_in_map_units_minus1 := bv.get_expGolomb()
	frame_mbs_only_flag := bv.get1BitBoolean()
	if !frame_mbs_only_flag {
		bv.skipBits(1) // mb_adaptive_frame_field_flag
	}
	bv.skipBits(1) // direct_8x8_inference_flag
	frame_cropping_flag := bv.get1Bit()
	if frame_cropping_flag != 0 {
		bv.get_expGolomb() // frame_crop_left_offset
		bv.get_expGolomb() // frame_crop_right_offset
		bv.get_expGolomb() // frame_crop_top_offset
		bv.get_expGolomb() // frame_crop_bottom_offset
	}
	vui_parameters_present_flag := bv.get1Bit()
	if vui_parameters_present_flag != 0 {
		this.analyzeVUIParameters(bv)
	}
}

func (this *H264VideoStreamParser) analyzeSEIData() {
	// Begin by making a copy of the NAL unit data, removing any 'emulation prevention' bytes:
	sei := make([]byte, SEI_MAX_SIZE)
	seiSize := this.removeEmulationBytes(sei, SEI_MAX_SIZE)

	var j uint = 1 // skip the initial byte (forbidden_zero_bit; nal_ref_idc; nal_unit_type); we've already seen it
	for j < seiSize {
		var payloadType uint
		for sei[j] == 255 && j < seiSize {
			j++
			payloadType += uint(sei[j])
		}
		if j >= seiSize {
			break
		}

		var payloadSize uint
		for sei[j] == 255 && j < seiSize {
			j++
			payloadSize += uint(sei[j])
		}
		if j >= seiSize {
			break
		}

		j += payloadSize
	}
}

func (this *H264VideoStreamParser) analyzeVUIParameters(bv *BitVector) {
	aspect_ratio_info_present_flag := bv.get1Bit()
	if aspect_ratio_info_present_flag != 0 {
		aspect_ratio_idc := bv.getBits(8)
		if aspect_ratio_idc == 255 /*Extended_SAR*/ {
			bv.skipBits(32) // sar_width; sar_height
		}
	}
	overscan_info_present_flag := bv.get1Bit()
	if overscan_info_present_flag != 0 {
		bv.skipBits(1) // overscan_appropriate_flag
	}
	video_signal_type_present_flag := bv.get1Bit()
	if video_signal_type_present_flag != 0 {
		bv.skipBits(4) // video_format; video_full_range_flag
		colour_description_present_flag := bv.get1Bit()
		if colour_description_present_flag != 0 {
			bv.skipBits(24) // colour_primaries; transfer_characteristics; matrix_coefficients
		}
	}
	chroma_loc_info_present_flag := bv.get1Bit()
	if chroma_loc_info_present_flag != 0 {
		bv.get_expGolomb() // chroma_sample_loc_type_top_field
		bv.get_expGolomb() // chroma_sample_loc_type_bottom_field
	}
	timing_info_present_flag := bv.get1Bit()
	if timing_info_present_flag != 0 {
		//num_units_in_tick := bv.getBits(32)
		//time_scale := bv.getBits(32)
		//fixed_frame_rate_flag := bv.get1Bit()
	}
}

func (this *H264VideoStreamParser) afterGetting()      {}
func (this *H264VideoStreamParser) doGetNextFrame()    {}
func (this *H264VideoStreamParser) stopGettingFrames() {}
func (this *H264VideoStreamParser) maxFrameSize() uint { return 0 }
func (this *H264VideoStreamParser) GetNextFrame(buffTo []byte, maxSize uint,
	afterGettingFunc, onCloseFunc interface{}) {
}

//////// H264VideoStreamFramer ////////
type H264VideoStreamFramer struct {
	MPEGVideoStreamFramer
	parser               *H264VideoStreamParser
	nextPresentationTime Timeval
	lastSeenSPS          []byte
	lastSeenPPS          []byte
	lastSeenSPSSize      uint
	lastSeenPPSSize      uint
	frameRate            float64
}

func NewH264VideoStreamFramer(inputSource IFramedSource) *H264VideoStreamFramer {
	h264VideoStreamFramer := new(H264VideoStreamFramer)
	h264VideoStreamFramer.parser = NewH264VideoStreamParser()
	h264VideoStreamFramer.inputSource = inputSource
	h264VideoStreamFramer.frameRate = 25.0
	h264VideoStreamFramer.InitMPEGVideoStreamFramer(h264VideoStreamFramer.parser)
	h264VideoStreamFramer.InitFramedSource(h264VideoStreamFramer.parser)
	return h264VideoStreamFramer
}

func (this *H264VideoStreamFramer) getSPSandPPS(sps, pps string, spsSize, ppsSize uint) {
	sps = string(this.lastSeenSPS)
	pps = string(this.lastSeenPPS)
	spsSize = this.lastSeenSPSSize
	ppsSize = this.lastSeenPPSSize
}

func (this *H264VideoStreamFramer) setSPSandPPS(sPropParameterSetsStr string) {
	sPropRecords, numSPropRecords := parseSPropParameterSets(sPropParameterSetsStr)
	var i uint
	for i = 0; i < numSPropRecords; i++ {
		if sPropRecords[i].sPropLength == 0 {
			continue
		}

		nalUnitType := (sPropRecords[i].sPropBytes[0]) & 0x1F
		if nalUnitType == 7 { /* SPS */
			this.saveCopyOfSPS(sPropRecords[i].sPropBytes, sPropRecords[i].sPropLength)
		} else if nalUnitType == 8 { /* PPS */
			this.saveCopyOfPPS(sPropRecords[i].sPropBytes, sPropRecords[i].sPropLength)
		}
	}
}

func (this *H264VideoStreamFramer) saveCopyOfSPS(from []byte, size uint) {
	this.lastSeenSPS = make([]byte, size)
	this.lastSeenSPS = from
	this.lastSeenSPSSize = size
}

func (this *H264VideoStreamFramer) saveCopyOfPPS(from []byte, size uint) {
	this.lastSeenPPS = make([]byte, size)
	this.lastSeenPPS = from
	this.lastSeenPPSSize = size
}

func (this *H264VideoStreamFramer) setPresentationTime() {
	this.presentationTime = this.nextPresentationTime
}
