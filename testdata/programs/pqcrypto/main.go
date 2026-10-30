// Post-quantum and modern public-key cryptography from the standard library:
// ML-KEM (crypto/mlkem), HPKE with the ML-KEM-768 + X25519 hybrid
// (crypto/hpke), SHA-3 and SHAKE, X25519 and HKDF. Only deterministic results
// are printed: keys come from fixed seeds, and the ciphertexts that are opened
// were produced by native Go (Go 1.27) beforehand with the same seeds, so
// goesm has to decapsulate and decrypt what gc encapsulated and encrypted.
package main

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/mldsa"
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"crypto/sha256"
	"crypto/sha3"
	"encoding/hex"
	"fmt"
)

// Ciphertexts produced by native Go for the seeds below.
const (
	mlkem768CT   = "695a60d9c79f08343ed9ff5802582063c2ca3a648e543d924affbb39ef4de656591f0d7689e6626be7ea7fedaf134e2c27c6797c73a5edaf16808f141c8afcf31614e8ab665379573e4d0a2037cbf776048167ba53576001a2596402cf24b5d45362bc893ceaef3599f76b10812e626002e66db5c5b0f2b9a7080e32db68dcc8d04c24f8461a58bb7e47efe670d740ad8af9820033845ef5f880f26f0e00adb2abef876f5270477ebbb02de6787ce72ca8785fb181f46c3ff7ae3787c25c68ccceefb3551875b9d77c4d439b6050eb382aacf9e744227e8c46e0a9a55838ea7034f5b4bcb61f1023a80186e795f4b3d8ae93988994224fa2d83e21711670da01e2b3e272f81616c0bc88cc46f641d16e0d0c0924cf4a4a5c1a9128c226d4918aa39bef94199dfffa33876ef0bfa0d9560d25f5ba08068d5271f32d2f9d88bcf53c7dcf811a8d5efe617f5e05700d3478d3cb7932528d1bceb240198a4cf8752caea3d387f00759a1356b7a5bf1838d26c3573e92e69f0f57c06e8c25459eb83e12cdd75f541a81ce710eafce2984783f30e37b327ff93b72297c6cd8c78c185ad53864952069d7d6c3bc633ae5e1a5925855df0b7e714bbde245f68822e0950c23c96d6111753a6ed0c46cce437f53b6bb708c1a3e25979733198d9879e3237e769471f922e579f37cfd641d29bdcfdbaa81edae09aeb046366e0376d04282d17778a8d54774e8c9be3c822b1e90cd8895abc1db8951b7687f63fee50ec43faf23730b15189e7c982b22d896a972da3c2ee529bb5fe63630c9c2ddfb9d1e4263a3d49af2832053d97efa2bd1782f25d7b864d6fb3708bfb9d4bc6c2cc6458d4f1459995db387e8b503825a4496c735252aa630a1bcaa7a2674727396dcaf67030b53473951651dc26c22476bfd11d33206af0ff035ed035e34716c905e8ddf043a4cdae145238d8f612dbcb75e879653bb9e2657dab58b944ff34f977fe15ce907f6814a5f92338774e6f2ab5257d24917decdd158c6d4594189f42a9b7fa9159a8af6aa825ba904654e08c894901298ffb27239ddea8283dd45b876036c0aecf03583ba444529757444c857fff6e4f8ed48f8a180adea54979a678f16dc6ac8edcc8e72ed08e96082f0ff4520dc635d4a846a3026fd86a48b1297e0cdfc06008793e783bde1c3fc6a71871e66b1feb560495817aabbdc59f0149f3e76add9b5bd6ce34734de7593ed607efb84c6e732960c744c908a9cb8947375a55b55fa2f0cd6742b75c10f65522d3844bed9b05bd441bbbea17cfbabdaef9847a0edd9c8329a762e34e5396014d88b4d344f250aaddefd917bb2120d1169c79cb09f59bad21850752c1099fff98b71bcdaab76f7063323e78faa521cd243f74ddc7f7775aa79960622e13580a6831e69bb7f2321d141d35da88317719078d4db319f308594c26836503f62362c40005022937c1298a928c040879661349a7b5362d0a75f2893b97a2600d5337239a70a6b64a457e6dfd5c74d462e7e790bb9ef3cee1461"
	mlkem768SS   = "9cddd089ffe70e3996e76f7c8d06746df34d07e8657bc0fcf2bb0e1c3084aea1"
	mlkem1024CT  = "3ce00ba803c544b01150a120e4afe3c9f42df12ac1502c16e38895f7a99b5730fca6f8fbc5e07c9e9d0e1a41e1b53ec6157770e34864d67f6faff1ccc5ebe7b521b79673aedb5565c41fd01d7d70783c9b358772f75408a7de50ff3c0f9035b3ea2813d2f7eb040f5a1f75e9f1c4f375c323dbc34a6a7c2b2509bd839e6ad4efe08a2310b51bb871bd3ebabe9aea8b72aa72494188500906d1d4cac87fa342a3c03e5ea8895ab431198627309d6825f4f852e2dfe98b4617bc4a58be50bccde430f7ca93c87439e5301c94166755666990eef4674ca3861f23bee45c72c86f0d3e9edacef6434bcd30f284f6eb551555c3c3d79ff2a782787ef6e825a4a891d9b6f88da68cf3a96bc6cbb14880b9f744b71b6d548f17073076b0369fc35f8dd30979003348448115b094ac44c66fe6842e55ca134118be4108d836fcee73e1d9bc650f94b7b944cf7d1f0a6344712d3f290457b26a0b9860cc012b71dc2bccd1710736d4d69b2d9c6a2d2f4e7639979ca805303dc283e055d8caad9edf08bc6bdfd2207b3f26e35f1f79361505d224bc18f36d688d2dbcefeb12cc98f2f7755b3b9974f80be19cdb9c8a7468cf0218b1f1173871c5b491119ddaae7a84327d7952c0d9c09100d3bbec74d141b386bd03a4003be055db61260e99f74c17f6de4cc8ea7860c970356cbf656fbd39521ee0620444365de5d8100f41f40820b108f430eabdf3beee999bd29d39733268cfb9670fbb4f6c28d41aa9012b6f6129231a37ab8c7f9cd4e2c52a8da6f76806e1df416999b11bcdc3e57e15418dbe04fe2857c41411ca5ac4871aefa2645868a84739f8bc481d4aa674216df2039409bd588d78c62ef993592da86abdb2560ce3d8f2bd48ab0a74418ccc75ccf0331faf1f2712cbb3d1ce0a4260b6487ecf1e05b6b8669df99d51b8cca088d4cd41ccf624347bcfb36a9883f7504710e246683b09ddc2a1dee31ceac2376609871085b057b2f1cc41e5e74c76b01d4ce796968a16e8e08c2c7497817d9d1bd1063daf5790bb331d25f6a434bff373d98719c555a640e983d8aef3a27cbdb58627fc2543088af450ddc4a2dae1936dbba16d6f6a6738ed51a36b93560623245547836ab6e5801bc854d6319ce486c634de2102e5f2006e5634b482e5cfcfd21a4953440267127a42fe7ff3f768d42124a9c98b3b513ebbc76740e06a1511d3e79765c2f9abbd4c12a10da8f0abf3f5062318b2104ac1c40866ae03ddbdf972ab7baeb0974a5db2a609dd0c02701f72db015fcfff6e451f49ee459cadc0be64f86803d595bbeab4f41ee7db7a5647f36e9b26861e21f72f3f1444f299833aefe6dbf6f8bb394d9a25081eb17d0fb8ac5955dd809cbec31eeeb45ba3390184ae25e16f5d4dbc53203f10d1ebf8a5799f2009486914537b2b15114f8b701b3727209d621c4f885c1b81f238d8e7c8e5c943d47f31090247856cc8bc0dc3392adc7c4c6b56e21ec8f59962b00617248735370e1888f3ab2145d42c1191fa70df98f157e4692dddf939a62bcb3d0599f73292b8e88456abf8cfd51a4271ea46b56a6c495c47d553a4acba045d3007dfeb8ae4be4f00c20e16bbc21cbbfc904a95de945c922a609a208ebb5a05668e601ce661b701c4ec565457a6c5124f7eea9420a80d81e1230ebe7e89ad78e4e2514eaf0f262416692f0711329762d1202b625fe2987539ca514947bb5e8895df355818c18e8f673246533b405f98e33c111cb4772fa16fed409b290ceb4f61240a8541ab78b461823f44cb5756d7539c5ce0f276c64d741ce9ba64c2d0377da5c4f2624c5afbebb4f93d8e7c6686e206adedb910b8e0185e29b47095e1d65c2c908087efafcf71fe852b0462b99a2628595b97eb0eb2d9c670e2d7ed708151bbc2f2305e35cb3ba1ac99ab9fc65c8e07922984df8dc144bbcb477045a663452c7bc989b3b46a84e00f92c6348b01f522ab60e47ef2863991d0022ed6304c7f3954ab61490d736aa5c2d1bd813b8461341704355ebaf8e70246954b5ab42f07293486a1835e39a8754c47a310ad37e2dcaa6849b80b47722b15cad6ecdac2c060f37b4fd506a08fbf6888ef70a3a9cfb79925db3193b514c17cb44d214978055577364099f016933ed9858273222b646e1db152b0004df451f503c499642a26c80e0da50e149be1bca32955093a8e2b42ed2b73b5484efc36c7"
	mlkem1024SS  = "0ad8d1ea1b8dd788979b4379581218df9321bdce5567eca42ae6be7d395f1a54"
	hpkeHybrid   = "1f26144a90245d6e7796eaf22e5d65536280df431812b50ac16307787bffe594187176bc318bdbfd73d8bef3a5b24b8d914a255888296d58f37a7fab20aa966007b14da791d076c66ce3e289ac435e39e5b6cc3f8419d259209880f9abf8770fbd9b9ac1a654d40e87d105855d12cc474e30bfd19af231571373d2fb1ef84d00350b801d679f4874a97e30a2df9b22d330b21d8bfe30dd85de7b2ddce9dc2044852ddab9af5d0655a8f9473cb26987467a91e1f0afb986811f820cc8793dba4977d0ec3ed057f46e561bcc3d4b5208727fdb12d541d061da2f7b981ecb62f81da29f5d30f0ff8c3b6cfbdac2415b78ec7ea336226d71a17a69b99575c44840f519192a9fa95a38731b2814a448052c737c8ae87635e60e5a18159d8b55e4f21465c5987599f14b3695952a5ffff5aee94318817a2a0a7e14a1c5f8c3e3d7d2bb1258460350933a6f92655522cc99c4c9475360f422063f83a5a7faa5425e3124b5722e869a897c6a6dff8a763a852812d9099edc1c26ade9c6a2d236f1d81e770ede19af7975934e57b60acc72c1abe709658ee81e39dded859c20d67e3053e5e1e3e8d2f273c915dc140397d894ac63b3925e484421e662a5e45225274131c26adc18293e74ca4a84e8132ba1809e5bc61f8be7edba3313cd3fc13a6cb104557d2e6536f787cdcde2f2fce13ff5f9addc8dee240491660b3b80fab38a3c5c72ae39a0baec60e8b982fca9ba09d12ed7b0f22267534445c2b35527a9c37fc119630dfd2b14a8efaf598566c6dc3f08ad3d95da1c6e7ed2a634c9cb19b53c535e3aee3b6f632c5cb02f5f31d8bd436ccb3883006ecbe7eb88a1683f64f4eb1af65c65dd5c4571af014b6a999e21e1d67550f820ba75de4e8557376b3fda647034c98b45aa6a286fc8dcb586fb01faab7e738782e1a8b324253162f280bc157f388f4281d06c70abc88466ab9a79b683c2dfac11558308e6c2fc5cca14fe6dfd68e28425cfdb4adb6b470a7c50c7f98300f4195051e446d59968f6e0978ae220563ab0d0d9eaf45861b83c9754cb12060d198442b49e6ad5a53a23f29d80f02dc933eeb1d80226ec481a85ea0576d4634cb04c562562579259b332ebcafcb49548d1f4edef00637e42e0747858b070f6e404598ad3962f38e46bfd573752b58285129d970c3791bd78c6b06805e02abd91cfb1537fabb5bc9ce0a515eb726118d72a280a77f357b903d99e99e1d57e4374a0f5bb129adc344183d68d89002391dbe3a7e090c9ff9ff26e03eef7723a32bb11ad7bcb6c888ba23e16c9ed6484a55b84f5fff4630a02014df3c2ba6dc5d0d86175506e98d4a36a66a1aaa41a0fea7b616dd70e843e3701ea9b66c53ef76d6d31b784e38f2a9876cc4d2d85d724115f96b9de301dba622fd0e9c584a56d0e11015fcf40e2a973aa977b00f293ef9fb225eb769e25d5765c84ca6e1b5f73700318ec09f0be57ee7ebdd6cb452d9fc9a2449e1bea74511d53740af419152ce1f00f163c34918aa360cdc247b72f66d60b9fb9b51144b2c7e8a8ace3ff07af9cc50ea2d7249bdaca00cb1f5d5104b25f01afe2d5b8d70522d88da8ef5d6cd3295d4356cb83c90d6f9d905d0d602984627768ec9227b18fd01f99b7ef623ca2627f8c"
	hpkeX25519   = "853dd510ca3f3801c17970aa18dd19e7ee49a7ebb35ae7a759fc5a93a45c9628ecfc724a10c202351ca44ff628efccd94ae6b53cbd90a5f7fc31878a51a96125b87991bd4a398e1157febe4f"
	hpkeMLKEM    = "4b956c36c9e6528db9d56a7649515e4cdb7813a6c94835b42a5872ce842e07444d3f1dd3b350eb611dc7a50d232b3a6ea862072a7008ad696402bcfa14b7c19fe712ca83308403bca3a05684e8dbd2d1e158c9b542c4f9154f558e2cf1aabd77491ce5cc2b995c1847d164845324975e677ec6e3046b60bf5546a0546bbe129c96df31b855d57405a28718815b5584b68cca971a0d66ce814ff3cfe3f4884edfd573e06daad1385d33c79b74c686c1dc4675015abcc20d89aeba4f60b8bf677eed9e7a86170cddeaa6d0284f8d16dce2dab7b071f0f0c4151ddd01e5bfe16552594d4542bfa7dbcb3c8427e8bd5327f2fd38dbd0866b075a67cd88962a6100f76a4523e76439ff9c220efce5b26ec6384e8f4deeb564b41d0fe775d92b588e65b714b8b3fa0d3a903378cd065c110fbb532fc8b0b6195d05a998e68af1450b9dbad11d9308cbbffa500d0dc3b2231c7076ce65b78055633962afe874fd7f3a9be3d39bdbfef019957fa5768a0af1c622f3efa83b6ceaa2006f057d7bf27efaab9f62674bdb06fca79165b77f68b6116e1d69bc0ef9427edad7f8db724917fdfea82546be75c989a074679d21d0b447678f782f9c8a5511e306e0f6dcbaba16703bb08485e44f3c228f6d546f6290f6096254e9af048e6e20141525756b9de88339679f2f478ba2411af1e6bb5ac9653cad4eb3809c614e389f0bb746ecbd0f867f75dba7f09830ccdbba8ca9fc6496c7d773dba0a4137e4222a7c8b810e2b744b7cbeaf7aecf89bb823d6cd89ceef6be742c62a03197f5eb74dce47fec1578f7712ba4e2c8c03e36288be29034bfd380de8bf11e68f9347b1ab4eb282af32a461d02966057b887ab33a80082f5a0c380436f45c31160e4b5e38696acbf4da4ca1d472a7eb4fe4b6c0827d31a8e2ba7a79c00017bd996ab3422813d71f5bc0b0b24ab96c9c08ec1bb34d05b2b4e99a50028c497bed21436a9c5866566f029e145918336441819fc74836b9f1b45a11fc1fa0c0945c4eda09d8455469f708ce193b14a519497b2565c32772def56d1ffcb5424edeecd1cb659728ebf22a8a7b8c02325b54d87279fe0da508becb5bd5f7391366b977f275e70101ca1e2afd905455763bb4edc184e506ae07dfadcda47dddcd213476c2b73b649361f84777b82f532aa0e34b5c03067c9e0655afe289869a5747a0e402fd13bc22878d6414d032d25fdef43c110962e62ca914436bf1f51d0f01b61a199a7507408e7ae4a9b21a16746d1e794c4d44e37f03c3450cddd2224324d96a747c730a46251538bdbbb955f3e16cc7952a3ffb02e76495a498c37affbde0fd7ccadc04e659d578e8f24210ca19988b39fbb0dbce06bf9d82cb6dd50557a57d8c1fa00926b7b5e624d45e0aab2b6ca2bbd66e2cf4e659c784a40c1c89b775ad06929faf9fc3acf3003ad0370d6289ad8d8080cd52e9aa96dc4148df3b3b7147a36b2891fbaa2966b8fed6ab9638d9905d6546c850788c6d6aad556dad409a5ff069561d557964a034c67f0a151864297c49a41bd7f8ade5b0e2c225fbc16dbf6403da35f70f174251bedb9d7434b9924af939e813c"
	hpkeHybridPT = "post-quantum hello from native Go"
)

func seq(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}

func unhex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	// ML-KEM-768 from a fixed 64-byte seed.
	dk, err := mlkem.NewDecapsulationKey768(seq(0, 64))
	check(err)
	ek := dk.EncapsulationKey()
	fmt.Println("mlkem768 seed:", bytes.Equal(dk.Bytes(), seq(0, 64)))
	fmt.Println("mlkem768 ek:", len(ek.Bytes()), digest(ek.Bytes()))
	ss, err := dk.Decapsulate(unhex(mlkem768CT))
	check(err)
	fmt.Println("mlkem768 decapsulate native ciphertext:", hex.EncodeToString(ss), hex.EncodeToString(ss) == mlkem768SS)
	ss2, ct2, err := mlkemtest.Encapsulate768(ek, seq(0x40, 32))
	check(err)
	fmt.Println("mlkem768 deterministic encapsulate:", bytes.Equal(ct2, unhex(mlkem768CT)), bytes.Equal(ss2, ss))
	// A modified ciphertext decapsulates to a pseudorandom key (implicit
	// rejection) that depends on the secret key.
	bad := unhex(mlkem768CT)
	bad[100] ^= 1
	ssBad, err := dk.Decapsulate(bad)
	check(err)
	fmt.Println("mlkem768 implicit rejection:", hex.EncodeToString(ssBad))
	// Encapsulation key round trip, and a random encapsulation.
	ek2, err := mlkem.NewEncapsulationKey768(ek.Bytes())
	check(err)
	rss, rct := ek2.Encapsulate()
	dss, err := dk.Decapsulate(rct)
	check(err)
	fmt.Println("mlkem768 random round trip:", len(rct), bytes.Equal(rss, dss))
	_, err = mlkem.NewEncapsulationKey768(ek.Bytes()[:100])
	fmt.Println("mlkem768 short key:", err)
	_, err = dk.Decapsulate(rct[:100])
	fmt.Println("mlkem768 short ciphertext:", err)

	// ML-KEM-1024.
	dk1, err := mlkem.NewDecapsulationKey1024(seq(0, 64))
	check(err)
	fmt.Println("mlkem1024 ek:", len(dk1.EncapsulationKey().Bytes()), digest(dk1.EncapsulationKey().Bytes()))
	ss, err = dk1.Decapsulate(unhex(mlkem1024CT))
	check(err)
	fmt.Println("mlkem1024 decapsulate native ciphertext:", hex.EncodeToString(ss) == mlkem1024SS)

	// ML-DSA: deterministic signatures are compared bit for bit.
	for _, params := range []mldsa.Parameters{mldsa.MLDSA44(), mldsa.MLDSA65()} {
		sk, err := mldsa.NewPrivateKey(params, seq(0x30, 32))
		check(err)
		pk := sk.PublicKey()
		sig, err := sk.SignDeterministic([]byte("goesm"), &mldsa.Options{Context: "ctx"})
		check(err)
		fmt.Println("mldsa:", len(pk.Bytes()), digest(pk.Bytes()), len(sig), digest(sig),
			mldsa.Verify(pk, []byte("goesm"), sig, &mldsa.Options{Context: "ctx"}),
			mldsa.Verify(pk, []byte("goesm"), sig, nil) != nil,
			mldsa.Verify(pk, []byte("goesn"), sig, &mldsa.Options{Context: "ctx"}) != nil)
	}

	// SHA-3 and SHAKE.
	msg := []byte("goesm post-quantum")
	for _, n := range []int{0, 1, 135, 136, 137, 300} {
		m := bytes.Repeat([]byte{0xa5}, n)
		a := sha3.Sum256(m)
		b := sha3.Sum512(m)
		fmt.Println("sha3", n, hex.EncodeToString(a[:6]), hex.EncodeToString(b[:6]))
	}
	s128 := sha3.SumSHAKE128(msg, 40)
	s256 := sha3.SumSHAKE256(msg, 40)
	fmt.Println("shake128:", hex.EncodeToString(s128))
	fmt.Println("shake256:", hex.EncodeToString(s256))
	x := sha3.NewSHAKE256()
	x.Write(msg[:5])
	x.Write(msg[5:])
	out1 := make([]byte, 7)
	out2 := make([]byte, 173)
	x.Read(out1)
	x.Read(out2)
	fmt.Println("shake256 streamed:", hex.EncodeToString(out1), digest(out2))

	// X25519 (RFC 7748 section 6.1).
	alice, err := ecdh.X25519().NewPrivateKey(unhex("77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"))
	check(err)
	bob, err := ecdh.X25519().NewPrivateKey(unhex("5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb"))
	check(err)
	shared, err := alice.ECDH(bob.PublicKey())
	check(err)
	fmt.Println("x25519 alice pub:", hex.EncodeToString(alice.PublicKey().Bytes()))
	fmt.Println("x25519 shared:", hex.EncodeToString(shared))

	// HKDF (RFC 5869 test case 1).
	okm, err := hkdf.Key(sha256.New, bytes.Repeat([]byte{0x0b}, 22), seq(0, 13), string(seq(0xf0, 10)), 42)
	check(err)
	fmt.Println("hkdf:", hex.EncodeToString(okm))

	// Many keys, from a SHAKE128 stream standing in for a random source: the
	// field arithmetic, the sampling and the re-encryption check of
	// decapsulation (also of corrupted ciphertexts) all feed one digest.
	rng := sha3.NewSHAKE128()
	rng.Write([]byte("goesm pqcrypto"))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		rng.Read(b)
		return b
	}
	acc := sha3.New256()
	for i := 0; i < 24; i++ {
		k, err := mlkem.NewDecapsulationKey768(rnd(64))
		check(err)
		ss, ct, err := mlkemtest.Encapsulate768(k.EncapsulationKey(), rnd(32))
		check(err)
		back, err := k.Decapsulate(ct)
		check(err)
		ct[int(rnd(1)[0])*4%len(ct)] ^= byte(1 + i)
		other, err := k.Decapsulate(ct)
		check(err)
		fmt.Fprint(acc, bytes.Equal(ss, back), bytes.Equal(ss, other))
		acc.Write(k.EncapsulationKey().Bytes())
		acc.Write(ss)
		acc.Write(ct)
		acc.Write(other)
	}
	for i := 0; i < 6; i++ {
		k, err := mlkem.NewDecapsulationKey1024(rnd(64))
		check(err)
		ss, ct, err := mlkemtest.Encapsulate1024(k.EncapsulationKey(), rnd(32))
		check(err)
		back, err := k.Decapsulate(ct)
		check(err)
		ct[int(rnd(1)[0])*6%len(ct)] ^= byte(1 + i)
		other, err := k.Decapsulate(ct)
		check(err)
		fmt.Fprint(acc, bytes.Equal(ss, back), bytes.Equal(ss, other))
		acc.Write(k.EncapsulationKey().Bytes())
		acc.Write(ss)
		acc.Write(other)
	}
	for i := 0; i < 12; i++ {
		a, err := ecdh.X25519().NewPrivateKey(rnd(32))
		check(err)
		b, err := ecdh.X25519().NewPrivateKey(rnd(32))
		check(err)
		s1, err := a.ECDH(b.PublicKey())
		check(err)
		s2, err := b.ECDH(a.PublicKey())
		check(err)
		fmt.Fprint(acc, bytes.Equal(s1, s2))
		acc.Write(a.PublicKey().Bytes())
		acc.Write(s1)
	}
	fmt.Println("many keys:", hex.EncodeToString(acc.Sum(nil)))

	// HPKE: ML-KEM-768 + X25519 (X-Wing), HKDF-SHA256, AES-256-GCM.
	info := []byte("goesm info")
	hy := hpke.MLKEM768X25519()
	hsk, err := hy.DeriveKeyPair(seq(0x80, 32))
	check(err)
	fmt.Println("hpke hybrid kem:", fmt.Sprintf("%#04x", hy.ID()), digest(hsk.PublicKey().Bytes()), len(hsk.PublicKey().Bytes()))
	pt, err := hpke.Open(hsk, hpke.HKDFSHA256(), hpke.AES256GCM(), info, unhex(hpkeHybrid))
	fmt.Println("hpke hybrid open native ciphertext:", string(pt), err)
	tampered := unhex(hpkeHybrid)
	tampered[len(tampered)-1] ^= 1
	_, err = hpke.Open(hsk, hpke.HKDFSHA256(), hpke.AES256GCM(), info, tampered)
	fmt.Println("hpke hybrid open tampered:", err)
	_, err = hpke.Open(hsk, hpke.HKDFSHA256(), hpke.AES256GCM(), []byte("other info"), unhex(hpkeHybrid))
	fmt.Println("hpke hybrid open other info:", err)

	// HPKE with DHKEM(X25519), and ML-KEM-768 with SHAKE256 and ChaCha20-Poly1305.
	xsk, err := hpke.DHKEM(ecdh.X25519()).DeriveKeyPair(seq(0x90, 32))
	check(err)
	fmt.Println("hpke x25519 public:", hex.EncodeToString(xsk.PublicKey().Bytes()))
	pt, err = hpke.Open(xsk, hpke.HKDFSHA256(), hpke.AES128GCM(), info, unhex(hpkeX25519))
	fmt.Println("hpke x25519 open native ciphertext:", string(pt), err)
	msk, err := hpke.MLKEM768().DeriveKeyPair(seq(0xa0, 32))
	check(err)
	pt, err = hpke.Open(msk, hpke.SHAKE256(), hpke.ChaCha20Poly1305(), info, unhex(hpkeMLKEM))
	fmt.Println("hpke mlkem768 open native ciphertext:", string(pt), err)

	// Seal then open through every KEM, with the sender and recipient
	// contexts too (several messages, exporter secrets).
	kems := []hpke.KEM{
		hpke.MLKEM768X25519(), hpke.MLKEM768(), hpke.MLKEM1024(),
		hpke.MLKEM768P256(),
		hpke.DHKEM(ecdh.X25519()), hpke.DHKEM(ecdh.P256()),
	}
	for _, kem := range kems {
		sk, err := kem.DeriveKeyPair(seq(0x10, 32))
		check(err)
		pk := sk.PublicKey()
		sealed, err := hpke.Seal(pk, hpke.HKDFSHA256(), hpke.AES256GCM(), info, []byte("round trip"))
		check(err)
		back, err := hpke.Open(sk, hpke.HKDFSHA256(), hpke.AES256GCM(), info, sealed)
		fmt.Printf("hpke kem %#04x round trip: %s %v %d\n", kem.ID(), back, err, len(sealed))
		enc, snd, err := hpke.NewSender(pk, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), info)
		check(err)
		rcv, err := hpke.NewRecipient(enc, sk, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), info)
		check(err)
		c1, err := snd.Seal([]byte("aad"), []byte("first"))
		check(err)
		c2, err := snd.Seal(nil, []byte("second"))
		check(err)
		p1, err1 := rcv.Open([]byte("aad"), c1)
		p2, err2 := rcv.Open(nil, c2)
		e1, err := snd.Export("ctx", 20)
		check(err)
		e2, err := rcv.Export("ctx", 20)
		check(err)
		fmt.Printf("hpke kem %#04x stream: %s %s %v %v %v\n", kem.ID(), p1, p2, err1, err2, bytes.Equal(e1, e2))
	}

	// Every KDF with every AEAD, over the hybrid KEM.
	hsk2, err := hy.DeriveKeyPair(seq(0x20, 32))
	check(err)
	for _, kdf := range []hpke.KDF{hpke.HKDFSHA256(), hpke.HKDFSHA384(), hpke.HKDFSHA512(), hpke.SHAKE128(), hpke.SHAKE256()} {
		for _, aead := range []hpke.AEAD{hpke.AES128GCM(), hpke.AES256GCM(), hpke.ChaCha20Poly1305()} {
			sealed, err := hpke.Seal(hsk2.PublicKey(), kdf, aead, info, []byte("suite"))
			check(err)
			back, err := hpke.Open(hsk2, kdf, aead, info, sealed)
			fmt.Printf("hpke kdf %#04x aead %#04x: %s %v %d\n", kdf.ID(), aead.ID(), back, err, len(sealed))
		}
	}
}
